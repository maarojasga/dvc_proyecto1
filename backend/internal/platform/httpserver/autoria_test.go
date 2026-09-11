package httpserver_test

import (
	"net/http"
	"testing"
)

// moduloEnVersion crea un módulo y devuelve su identificador.
func moduloEnVersion(t *testing.T, c *cliente, versionID, titulo string) string {
	t.Helper()
	res := c.hacer(http.MethodPost, "/courses/versions/"+versionID+"/modules",
		map[string]any{"title": titulo, "position": 1})
	if res.Estado != http.StatusCreated {
		t.Fatalf("crear módulo: %d %s", res.Estado, res.Crudo)
	}
	id, _ := res.campo(t, "ID").(string)
	if id == "" {
		t.Fatalf("no se pudo leer el id del módulo: %s", res.Crudo)
	}
	return id
}

func TestUnaVersionPublicadaEsInmutable(t *testing.T) {
	env := nuevoEntorno(t)
	c, versionID := env.profesorConCurso("prof@example.com", "curso-inmutable")

	// Estructura mínima publicable: módulo, unidad y recurso visible.
	moduloID := moduloEnVersion(t, c, versionID, "Módulo 1")
	res := c.hacer(http.MethodPost, "/courses/versions/"+versionID+"/modules/"+moduloID+"/units",
		map[string]any{"title": "Unidad 1", "position": 1})
	if res.Estado != http.StatusCreated {
		t.Fatalf("crear unidad: %d %s", res.Estado, res.Crudo)
	}
	unidadID, _ := res.campo(t, "ID").(string)

	res = c.hacer(http.MethodPost, "/courses/versions/"+versionID+"/units/"+unidadID+"/resources",
		map[string]any{"type": "text", "title": "Lectura", "position": 1, "visible": true, "text_content_md": "# Hola"})
	if res.Estado != http.StatusCreated {
		t.Fatalf("crear recurso: %d %s", res.Estado, res.Crudo)
	}

	res = c.hacer(http.MethodPatch, "/courses/versions/"+versionID, map[string]any{
		"title": "Curso publicable", "summary": "Resumen",
		"approval_min_score": 60, "approval_required_resources_pct": 80,
	})
	if res.Estado != http.StatusOK {
		t.Fatalf("metadatos: %d %s", res.Estado, res.Crudo)
	}
	if res := c.hacer(http.MethodPost, "/courses/versions/"+versionID+"/publish", nil); res.Estado != http.StatusOK {
		t.Fatalf("publicar: %d %s", res.Estado, res.Crudo)
	}

	// Publicada la versión, ninguna operación de estructura debe alterarla.
	// Antes solo lo comprobaban los metadatos y el alta de módulos, así que un
	// curso vivo se podía reescribir entero.
	casos := []struct {
		nombre string
		metodo string
		ruta   string
		cuerpo any
	}{
		{"metadatos", http.MethodPatch, "/courses/versions/" + versionID,
			map[string]any{"title": "Cambiado", "summary": "x", "approval_min_score": 60, "approval_required_resources_pct": 80}},
		{"añadir módulo", http.MethodPost, "/courses/versions/" + versionID + "/modules",
			map[string]any{"title": "Otro", "position": 2}},
		{"editar módulo", http.MethodPatch, "/courses/versions/" + versionID + "/modules/" + moduloID,
			map[string]any{"title": "Renombrado", "position": 1}},
		{"borrar módulo", http.MethodDelete, "/courses/versions/" + versionID + "/modules/" + moduloID, nil},
		{"añadir unidad", http.MethodPost, "/courses/versions/" + versionID + "/modules/" + moduloID + "/units",
			map[string]any{"title": "Otra", "position": 2}},
		{"editar unidad", http.MethodPatch, "/courses/versions/" + versionID + "/units/" + unidadID,
			map[string]any{"title": "Renombrada", "position": 1}},
		{"borrar unidad", http.MethodDelete, "/courses/versions/" + versionID + "/units/" + unidadID, nil},
		{"añadir recurso", http.MethodPost, "/courses/versions/" + versionID + "/units/" + unidadID + "/resources",
			map[string]any{"type": "text", "title": "Otro", "position": 2}},
	}
	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			res := c.hacer(caso.metodo, caso.ruta, caso.cuerpo)
			if res.Estado != http.StatusConflict {
				t.Errorf("debería rechazarse con 409 sobre una versión publicada, llegó %d: %s", res.Estado, res.Crudo)
			}
		})
	}

	// Y la estructura sigue intacta.
	if n := env.contar(`SELECT count(*) FROM modules`); n != 1 {
		t.Errorf("el módulo no debería haberse borrado ni duplicado, hay %d", n)
	}
	if n := env.contar(`SELECT count(*) FROM modules WHERE title = 'Módulo 1'`); n != 1 {
		t.Error("el título del módulo no debería haber cambiado")
	}
}

func TestDespublicarDevuelveLaEdicion(t *testing.T) {
	env := nuevoEntorno(t)
	c, versionID := env.profesorConCurso("prof@example.com", "curso-despublicable")
	moduloEnVersion(t, c, versionID, "Módulo 1")

	cursoID := env.consultarTexto(`SELECT id::text FROM courses WHERE slug='curso-despublicable'`)
	// Se despublica sin haber publicado: la operación debe rechazarse, no
	// dejar el curso en un estado intermedio.
	if res := c.hacer(http.MethodPost, "/courses/"+cursoID+"/unpublish", nil); res.Estado == http.StatusOK {
		t.Error("no debería poder despublicarse un curso que nunca se publicó")
	}
	// Y el borrador sigue siendo editable.
	if res := c.hacer(http.MethodPost, "/courses/versions/"+versionID+"/modules",
		map[string]any{"title": "Módulo 2", "position": 2}); res.Estado != http.StatusCreated {
		t.Errorf("el borrador debería seguir siendo editable, llegó %d: %s", res.Estado, res.Crudo)
	}
}

func TestNoSePuedeTocarLaEstructuraDeOtroProfesor(t *testing.T) {
	env := nuevoEntorno(t)
	ana, versionAna := env.profesorConCurso("ana@example.com", "curso-de-ana")
	beto, versionBeto := env.profesorConCurso("beto@example.com", "curso-de-beto")

	moduloAna := moduloEnVersion(t, ana, versionAna, "Módulo de Ana")

	// Beto está autorizado sobre SU versión, pero pasa el identificador de un
	// módulo de Ana. Sin acotar la consulta por versión, la ruta autorizaba y
	// el UPDATE alcanzaba el módulo ajeno.
	casos := []struct {
		nombre string
		metodo string
		ruta   string
		cuerpo any
	}{
		{"editar módulo ajeno", http.MethodPatch,
			"/courses/versions/" + versionBeto + "/modules/" + moduloAna,
			map[string]any{"title": "Secuestrado", "position": 9}},
		{"borrar módulo ajeno", http.MethodDelete,
			"/courses/versions/" + versionBeto + "/modules/" + moduloAna, nil},
		{"colgar unidad de un módulo ajeno", http.MethodPost,
			"/courses/versions/" + versionBeto + "/modules/" + moduloAna + "/units",
			map[string]any{"title": "Intrusa", "position": 1}},
	}
	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			res := beto.hacer(caso.metodo, caso.ruta, caso.cuerpo)
			if res.Estado != http.StatusNotFound {
				t.Errorf("debería responder 404 como si no existiera, llegó %d: %s", res.Estado, res.Crudo)
			}
		})
	}

	// El módulo de Ana queda intacto.
	if n := env.contar(`SELECT count(*) FROM modules WHERE title='Módulo de Ana'`); n != 1 {
		t.Error("el módulo de Ana no debería haberse alterado ni borrado")
	}
	if n := env.contar(`SELECT count(*) FROM units`); n != 0 {
		t.Errorf("no debería haberse creado ninguna unidad, hay %d", n)
	}
}

func TestPublicarSinEstructuraDevuelveLaListaExhaustivaDeErrores(t *testing.T) {
	env := nuevoEntorno(t)
	c, versionID := env.profesorConCurso("prof@example.com", "curso-incompleto")

	res := c.hacer(http.MethodPost, "/courses/versions/"+versionID+"/publish", nil)
	if res.Estado != http.StatusUnprocessableEntity {
		t.Fatalf("%d %s", res.Estado, res.Crudo)
	}
	detalles, _ := res.campo(t, "error", "details").([]any)
	// La demostración pide una lista exhaustiva, no el primer error.
	if len(detalles) < 2 {
		t.Errorf("se esperaban varios motivos, llegó %v: %s", detalles, res.Crudo)
	}
	if n := env.contar(`SELECT count(*) FROM course_versions WHERE status='published'`); n != 0 {
		t.Error("no debería haberse publicado nada")
	}
}

func TestLaPrevisualizacionDevuelveElArbolCompleto(t *testing.T) {
	env := nuevoEntorno(t)
	c, versionID := env.profesorConCurso("prof@example.com", "curso-preview")
	moduloID := moduloEnVersion(t, c, versionID, "Módulo 1")
	c.hacer(http.MethodPost, "/courses/versions/"+versionID+"/modules/"+moduloID+"/units",
		map[string]any{"title": "Unidad 1", "position": 1})

	res := c.hacer(http.MethodGet, "/courses/versions/"+versionID, nil)
	if res.Estado != http.StatusOK {
		t.Fatalf("%d %s", res.Estado, res.Crudo)
	}
	modulos, _ := res.campo(t, "Modules").([]any)
	if len(modulos) != 1 {
		t.Fatalf("se esperaba 1 módulo en la previsualización: %s", res.Crudo)
	}

	// Y un profesor ajeno no puede previsualizar el borrador.
	env.registrarYVerificar("otro@example.com")
	env.asciendeA("otro@example.com", "teacher")
	otro := env.entrar("otro@example.com", clavePrueba)
	if ajeno := otro.hacer(http.MethodGet, "/courses/versions/"+versionID, nil); ajeno.Estado == http.StatusOK {
		t.Errorf("un profesor ajeno no debería ver el borrador, llegó %d", ajeno.Estado)
	}
}

func TestCargaMultipartCicloDeVidaYReanudacion(t *testing.T) {
	env := nuevoEntorno(t)
	c, versionID := env.profesorConCurso("prof-multi@example.com", "curso-multi")
	moduloID := moduloEnVersion(t, c, versionID, "Módulo Multimedia")

	res := c.hacer(http.MethodPost, "/courses/versions/"+versionID+"/modules/"+moduloID+"/units",
		map[string]any{"title": "Unidad Video", "position": 1})
	unidadID, _ := res.campo(t, "ID").(string)

	res = c.hacer(http.MethodPost, "/courses/versions/"+versionID+"/units/"+unidadID+"/resources",
		map[string]any{"type": "video", "title": "Clase Video", "position": 1, "visible": true})
	recursoID, _ := res.campo(t, "ID").(string)

	// 1. Iniciar subida multipart
	res = c.hacer(http.MethodPost, "/courses/versions/"+versionID+"/resources/"+recursoID+"/multipart/initiate",
		map[string]any{"content_type": "video/mp4"})
	if res.Estado != http.StatusOK {
		t.Fatalf("iniciar multipart: %d %s", res.Estado, res.Crudo)
	}
	uploadID, _ := res.campo(t, "upload_id").(string)
	if uploadID == "" {
		t.Fatalf("no se recibió upload_id: %s", res.Crudo)
	}

	// 2. Solicitar URL firmada de la parte 1
	res = c.hacer(http.MethodPost, "/courses/versions/"+versionID+"/resources/"+recursoID+"/multipart/part-url",
		map[string]any{"upload_id": uploadID, "part_number": 1})
	if res.Estado != http.StatusOK {
		t.Fatalf("pedir url de parte: %d %s", res.Estado, res.Crudo)
	}
	partURL, _ := res.campo(t, "upload_url").(string)
	if partURL == "" {
		t.Fatalf("no se recibió upload_url de la parte: %s", res.Crudo)
	}

	// 3. Consultar partes para reanudación de carga interrumpida
	res = c.hacer(http.MethodGet, "/courses/versions/"+versionID+"/resources/"+recursoID+"/multipart/parts?upload_id="+uploadID, nil)
	if res.Estado != http.StatusOK {
		t.Fatalf("listar partes para reanudar: %d %s", res.Estado, res.Crudo)
	}
}
