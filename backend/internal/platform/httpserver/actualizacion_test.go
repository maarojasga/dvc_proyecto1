package httpserver_test

import (
	"context"
	"net/http"
	"testing"
)

// Cubren el alcance opcional 5.2: borradores de actualización con
// clasificación de cambios y migración de progreso.

// cursoPublicadoConLectura deja publicado un curso con una lectura obligatoria
// y devuelve el cliente del profesor, el id del curso y el del recurso.
func (e *entorno) cursoPublicadoConLectura(correo, slug string) (c *cliente, cursoID, recursoID string) {
	e.t.Helper()
	c, versionID := e.profesorConCurso(correo, slug)
	unidadID := e.unidadVacia(c, versionID)

	res := c.hacer(http.MethodPost, "/courses/versions/"+versionID+"/units/"+unidadID+"/resources",
		map[string]any{"type": "text", "title": "Lectura", "position": 1,
			"visible": true, "required": true, "text_content_md": "# Hola"})
	if res.Estado != http.StatusCreated {
		e.t.Fatalf("crear recurso: %d %s", res.Estado, res.Crudo)
	}
	recursoID, _ = res.campo(e.t, "ID").(string)

	if res := c.hacer(http.MethodPatch, "/courses/versions/"+versionID,
		map[string]any{"title": "Curso", "summary": "Resumen"}); res.Estado != http.StatusOK {
		e.t.Fatalf("metadatos: %d %s", res.Estado, res.Crudo)
	}
	if res := c.hacer(http.MethodPost, "/courses/versions/"+versionID+"/publish", nil); res.Estado != http.StatusOK {
		e.t.Fatalf("publicar: %d %s", res.Estado, res.Crudo)
	}
	cursoID = e.consultarTexto(`SELECT id::text FROM courses WHERE slug = $1`, slug)
	return c, cursoID, recursoID
}

// borradorDeActualizacion abre la siguiente versión y devuelve su id.
func (e *entorno) borradorDeActualizacion(c *cliente, cursoID string) string {
	e.t.Helper()
	res := c.hacer(http.MethodPost, "/courses/"+cursoID+"/update-draft", nil)
	if res.Estado != http.StatusCreated {
		e.t.Fatalf("abrir borrador de actualización: %d %s", res.Estado, res.Crudo)
	}
	id, _ := res.campo(e.t, "ID").(string)
	return id
}

func TestUnBorradorReciennAbiertoNoTieneCambios(t *testing.T) {
	env := nuevoEntorno(t)
	c, cursoID, _ := env.cursoPublicadoConLectura("prof@example.com", "curso-act")
	borradorID := env.borradorDeActualizacion(c, cursoID)

	res := c.hacer(http.MethodGet, "/courses/versions/"+borradorID+"/changes", nil)
	if res.Estado != http.StatusOK {
		t.Fatalf("clasificación: %d %s", res.Estado, res.Crudo)
	}
	if alcance, _ := res.campo(t, "alcance").(string); alcance != "ninguno" {
		t.Errorf("un borrador recién copiado no debería traer cambios: %s", res.Crudo)
	}
	if cambios, _ := res.campo(t, "cambios").([]any); len(cambios) != 0 {
		t.Errorf("se listaron cambios: %s", res.Crudo)
	}
}

// Reescribir el contenido se lista, pero no altera lo que hay que completar.
func TestReescribirUnaLecturaSeClasificaComoMenor(t *testing.T) {
	env := nuevoEntorno(t)
	c, cursoID, _ := env.cursoPublicadoConLectura("prof@example.com", "curso-act")
	borradorID := env.borradorDeActualizacion(c, cursoID)

	// El recurso del borrador es otra fila con el mismo stable_id.
	nuevoRecursoID := env.consultarTexto(`
		SELECT res.id::text FROM resources res
		  JOIN units u ON u.id = res.unit_id
		  JOIN modules m ON m.id = u.module_id
		 WHERE m.course_version_id = $1`, borradorID)

	if res := c.hacer(http.MethodPatch, "/courses/versions/"+borradorID+"/resources/"+nuevoRecursoID,
		map[string]any{"title": "Lectura", "position": 1, "visible": true, "required": true,
			"text_content_md": "# Hola, revisado"}); res.Estado != http.StatusOK {
		t.Fatalf("editar recurso: %d %s", res.Estado, res.Crudo)
	}

	res := c.hacer(http.MethodGet, "/courses/versions/"+borradorID+"/changes", nil)
	if alcance, _ := res.campo(t, "alcance").(string); alcance != "menor" {
		t.Fatalf("alcance inesperado: %s", res.Crudo)
	}
	if !contiene(res.Crudo, "contenido") {
		t.Errorf("el detalle no explica qué cambió: %s", res.Crudo)
	}
}

// Añadir un obligatorio cambia la vara con que se mide a todos los inscritos.
func TestAgregarUnObligatorioSeClasificaComoMayor(t *testing.T) {
	env := nuevoEntorno(t)
	c, cursoID, _ := env.cursoPublicadoConLectura("prof@example.com", "curso-act")
	borradorID := env.borradorDeActualizacion(c, cursoID)

	unidadID := env.consultarTexto(`
		SELECT u.id::text FROM units u
		  JOIN modules m ON m.id = u.module_id
		 WHERE m.course_version_id = $1`, borradorID)

	if res := c.hacer(http.MethodPost, "/courses/versions/"+borradorID+"/units/"+unidadID+"/resources",
		map[string]any{"type": "text", "title": "Nueva lectura", "position": 2,
			"visible": true, "required": true, "text_content_md": "# Extra"}); res.Estado != http.StatusCreated {
		t.Fatalf("agregar recurso: %d %s", res.Estado, res.Crudo)
	}

	res := c.hacer(http.MethodGet, "/courses/versions/"+borradorID+"/changes", nil)
	if alcance, _ := res.campo(t, "alcance").(string); alcance != "mayor" {
		t.Fatalf("alcance inesperado: %s", res.Crudo)
	}
	agregados, _ := res.campo(t, "obligatorios_agregados").([]any)
	if len(agregados) != 1 {
		t.Errorf("no se identificó el obligatorio agregado: %s", res.Crudo)
	}
	if !contiene(res.Crudo, "tendrán que completarlo") {
		t.Errorf("el detalle no advierte del efecto sobre los estudiantes: %s", res.Crudo)
	}
}

// El corazón de la migración: publicar una actualización lleva a los ya
// inscritos a la versión nueva, conservando lo que llevaban hecho.
func TestPublicarUnaActualizacionMigraElProgresoDeLosInscritos(t *testing.T) {
	env := nuevoEntorno(t)
	c, cursoID, recursoID := env.cursoPublicadoConLectura("prof@example.com", "curso-act")
	estudiante := env.inscribir("ana@example.com", cursoID)

	// La estudiante completa la lectura en la versión 1.
	if res := estudiante.hacer(http.MethodPost, "/resources/"+recursoID+"/progress",
		map[string]any{"type": "open"}); res.Estado != http.StatusOK {
		t.Fatalf("abrir el recurso: %d %s", res.Estado, res.Crudo)
	}
	env.envejecerProgreso("ana@example.com")
	if res := estudiante.hacer(http.MethodPost, "/resources/"+recursoID+"/progress",
		map[string]any{"type": "close", "complete": true}); res.Estado != http.StatusOK {
		t.Fatalf("cerrar el recurso: %d %s", res.Estado, res.Crudo)
	}

	antes := estudiante.hacer(http.MethodGet, "/enrollments/"+cursoID+"/progress", nil)
	if hechos, _ := antes.campo(t, "required_completed").(float64); hechos != 1 {
		t.Fatalf("la lectura debería constar como completada: %s", antes.Crudo)
	}
	versionOriginal := env.consultarTexto(
		`SELECT course_version_id::text FROM enrollments WHERE course_id = $1`, cursoID)

	// El profesor publica una actualización que solo reescribe el contenido.
	borradorID := env.borradorDeActualizacion(c, cursoID)
	if res := c.hacer(http.MethodPost, "/courses/versions/"+borradorID+"/publish", nil); res.Estado != http.StatusOK {
		t.Fatalf("publicar la actualización: %d %s", res.Estado, res.Crudo)
	}

	// La inscripción se mueve a la versión nueva...
	versionNueva := env.consultarTexto(
		`SELECT course_version_id::text FROM enrollments WHERE course_id = $1`, cursoID)
	if versionNueva == versionOriginal {
		t.Error("la inscripción se quedó en la versión anterior: el material nuevo no le aparecería nunca")
	}
	if versionNueva != borradorID {
		t.Errorf("la inscripción apunta a %s y no a la versión publicada %s", versionNueva, borradorID)
	}

	// ...y lo que llevaba hecho sigue contando, porque el stable_id se conservó.
	despues := estudiante.hacer(http.MethodGet, "/enrollments/"+cursoID+"/progress", nil)
	if hechos, _ := despues.campo(t, "required_completed").(float64); hechos != 1 {
		t.Errorf("el progreso no sobrevivió a la actualización: %s", despues.Crudo)
	}
	if pct, _ := despues.campo(t, "required_percent").(float64); pct != 100 {
		t.Errorf("el porcentaje no se conservó: %s", despues.Crudo)
	}
}

// Tras una actualización mayor, lo hecho se conserva pero lo exigido crece:
// el porcentaje baja sin perder el avance anterior.
func TestUnaActualizacionMayorConservaLoHechoYAmpliaLoExigido(t *testing.T) {
	env := nuevoEntorno(t)
	c, cursoID, recursoID := env.cursoPublicadoConLectura("prof@example.com", "curso-act")
	estudiante := env.inscribir("ana@example.com", cursoID)

	if res := estudiante.hacer(http.MethodPost, "/resources/"+recursoID+"/progress",
		map[string]any{"type": "open"}); res.Estado != http.StatusOK {
		t.Fatalf("abrir: %d %s", res.Estado, res.Crudo)
	}
	env.envejecerProgreso("ana@example.com")
	estudiante.hacer(http.MethodPost, "/resources/"+recursoID+"/progress",
		map[string]any{"type": "close", "complete": true})

	borradorID := env.borradorDeActualizacion(c, cursoID)
	unidadID := env.consultarTexto(`
		SELECT u.id::text FROM units u JOIN modules m ON m.id = u.module_id
		 WHERE m.course_version_id = $1`, borradorID)
	if res := c.hacer(http.MethodPost, "/courses/versions/"+borradorID+"/units/"+unidadID+"/resources",
		map[string]any{"type": "text", "title": "Nueva lectura", "position": 2,
			"visible": true, "required": true, "text_content_md": "# Extra"}); res.Estado != http.StatusCreated {
		t.Fatalf("agregar: %d %s", res.Estado, res.Crudo)
	}
	if res := c.hacer(http.MethodPost, "/courses/versions/"+borradorID+"/publish", nil); res.Estado != http.StatusOK {
		t.Fatalf("publicar: %d %s", res.Estado, res.Crudo)
	}

	res := estudiante.hacer(http.MethodGet, "/enrollments/"+cursoID+"/progress", nil)
	total, _ := res.campo(t, "required_total").(float64)
	hechos, _ := res.campo(t, "required_completed").(float64)
	if total != 2 {
		t.Errorf("lo exigido debería ser 2 recursos, es %v: %s", total, res.Crudo)
	}
	if hechos != 1 {
		t.Errorf("lo ya hecho debería conservarse, es %v: %s", hechos, res.Crudo)
	}
}

// Editar un recurso no puede llevarse por delante el archivo que ya tenía:
// la clave del objeto y el estado de procesamiento los gobierna la ingesta,
// no el formulario de autoría.
func TestEditarUnRecursoConservaSuArchivoYSuEstado(t *testing.T) {
	env := nuevoEntorno(t)
	c, versionID := env.profesorConCurso("prof-edit@example.com", "curso-edit")

	recursoID := recursoParaCarga(t, c, versionID, "pdf", "Guía")
	base := "/courses/versions/" + versionID + "/resources/" + recursoID
	clave := "resources/" + recursoID + "/original"
	if res := c.hacer(http.MethodPost, base+"/upload-url", map[string]any{"mime_type": "application/pdf"}); res.Estado != http.StatusOK {
		t.Fatalf("url de subida: %d %s", res.Estado, res.Crudo)
	}
	env.almacen.ponerObjeto(clave, pdfDePrueba(64), "application/pdf")
	if res := c.hacer(http.MethodPost, base+"/confirm-upload", map[string]any{}); res.Estado != http.StatusOK {
		t.Fatalf("confirmar: %d %s", res.Estado, res.Crudo)
	}

	// Solo se corrige el título.
	if res := c.hacer(http.MethodPatch, base, map[string]any{
		"type": "pdf", "title": "Guía revisada", "position": 1, "visible": true, "required": true,
	}); res.Estado != http.StatusOK {
		t.Fatalf("editar el título: %d %s", res.Estado, res.Crudo)
	}

	guardado := env.consultarTexto(
		`SELECT coalesce(object_key,'') || '|' || processing_status FROM resources WHERE id = $1`, recursoID)
	if guardado != clave+"|ready" {
		t.Errorf("editar el título alteró el archivo o el estado: %q", guardado)
	}
}

// envejecerProgreso retrasa el último evento para superar la permanencia
// mínima sin que la prueba tenga que esperar de verdad.
func (e *entorno) envejecerProgreso(correo string) {
	e.t.Helper()
	if _, err := e.pool.Exec(context.Background(), `
		UPDATE progress_events SET occurred_at = occurred_at - interval '2 minutes'
		 WHERE enrollment_id IN (
		       SELECT en.id FROM enrollments en
		         JOIN users u ON u.id = en.student_id
		        WHERE u.email = $1)`, correo); err != nil {
		e.t.Fatalf("envejecer el progreso: %v", err)
	}
}
