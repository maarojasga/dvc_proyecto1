package httpserver_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/google/uuid"
)

// recursoEnCursoPublicado añade un recurso del tipo indicado a un curso ya
// publicado y devuelve su identificador.
//
// Se inserta directamente en base porque la versión publicada es inmutable, y
// lo que se quiere ejercitar aquí es la entrega, no la autoría.
func (e *entorno) recursoEnCursoPublicado(slug, tipo, objectKey, texto string) string {
	e.t.Helper()
	ctx := context.Background()

	unidadID := e.consultarTexto(`
		SELECT u.id::text FROM units u
		  JOIN modules m ON m.id = u.module_id
		  JOIN course_versions v ON v.id = m.course_version_id
		  JOIN courses c ON c.id = v.course_id AND c.current_published_version_id = v.id
		 WHERE c.slug = $1 LIMIT 1`, slug)

	recursoID := uuid.New()
	if _, err := e.pool.Exec(ctx, `
		INSERT INTO resources (id, unit_id, type, title, position, visible, required,
		                       downloadable, processing_status, object_key, text_content_md)
		VALUES ($1, $2, $3, $4, 9, true, false, true, 'ready', $5, $6)`,
		recursoID, unidadID, tipo, "Recurso "+tipo, nulo(objectKey), nulo(texto)); err != nil {
		e.t.Fatalf("insertar recurso %s: %v", tipo, err)
	}
	return recursoID.String()
}

func nulo(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func TestElPDFSeEntregaConSuURLAutorizada(t *testing.T) {
	env := nuevoEntorno(t)
	_, cursoID := env.cursoPublicadoConVideo("prof@example.com", "curso-mixto", "ready")
	pdfID := env.recursoEnCursoPublicado("curso-mixto", "pdf", "documentos/guia.pdf", "")
	estudiante := env.inscribir("ana@example.com", cursoID)

	res := estudiante.hacer(http.MethodGet, "/resources/"+pdfID+"/content", nil)
	if res.Estado != http.StatusOK {
		t.Fatalf("%d %s", res.Estado, res.Crudo)
	}
	if tipo := res.campo(t, "type"); tipo != "pdf" {
		t.Errorf("type = %v", tipo)
	}
	if url := res.campo(t, "url"); url != "https://cdn.pruebas.local/documentos/guia.pdf" {
		t.Errorf("url = %v", url)
	}
	if res.campo(t, "downloadable") != true {
		t.Error("el recurso se marcó descargable y debería reflejarse")
	}
}

func TestQuienNoEstaInscritoNoObtieneElPDF(t *testing.T) {
	env := nuevoEntorno(t)
	env.cursoPublicadoConVideo("prof@example.com", "curso-mixto", "ready")
	pdfID := env.recursoEnCursoPublicado("curso-mixto", "pdf", "documentos/guia.pdf", "")

	env.registrarYVerificar("intruso@example.com")
	intruso := env.entrar("intruso@example.com", clavePrueba)
	if res := intruso.hacer(http.MethodGet, "/resources/"+pdfID+"/content", nil); res.Estado != http.StatusNotFound {
		t.Errorf("sin inscripción no debería entregarse, llegó %d: %s", res.Estado, res.Crudo)
	}
}

func TestElTextoViajaComoMarkdownYSinURL(t *testing.T) {
	env := nuevoEntorno(t)
	_, cursoID := env.cursoPublicadoConVideo("prof@example.com", "curso-mixto", "ready")
	textoID := env.recursoEnCursoPublicado("curso-mixto", "text", "", "# Título\n\nCuerpo de la lectura.")
	estudiante := env.inscribir("ana@example.com", cursoID)

	res := estudiante.hacer(http.MethodGet, "/resources/"+textoID+"/content", nil)
	if res.Estado != http.StatusOK {
		t.Fatalf("%d %s", res.Estado, res.Crudo)
	}
	if md := res.campo(t, "markdown"); md != "# Título\n\nCuerpo de la lectura." {
		t.Errorf("markdown = %v", md)
	}
	// Un recurso de texto no vive en el almacenamiento: no debe firmarse nada.
	if url := res.campo(t, "url"); url != nil {
		t.Errorf("un texto no debería traer URL de objeto, llegó %v", url)
	}
}

func TestLaReproduccionSeReanudaDondeSeDejo(t *testing.T) {
	env := nuevoEntorno(t)
	videoID, cursoID := env.cursoPublicadoConVideo("prof@example.com", "curso-video", "ready")
	estudiante := env.inscribir("ana@example.com", cursoID)

	// Sin nada reportado, se empieza desde el principio.
	res := estudiante.hacer(http.MethodGet, "/resources/"+videoID+"/content", nil)
	if pos := res.campo(t, "position_seconds"); pos != nil && pos != float64(0) {
		t.Errorf("sin reportar nada la posición debería ser 0, llegó %v", pos)
	}

	if res := estudiante.hacer(http.MethodPut, "/resources/"+videoID+"/position",
		map[string]int{"position_seconds": 372}); res.Estado != http.StatusNoContent {
		t.Fatalf("guardar posición: %d %s", res.Estado, res.Crudo)
	}

	res = estudiante.hacer(http.MethodGet, "/resources/"+videoID+"/content", nil)
	if pos := res.campo(t, "position_seconds"); pos != float64(372) {
		t.Errorf("debería reanudarse en 372, llegó %v", pos)
	}

	// Reportar de nuevo reemplaza la posición, no la acumula.
	estudiante.hacer(http.MethodPut, "/resources/"+videoID+"/position", map[string]int{"position_seconds": 12})
	res = estudiante.hacer(http.MethodGet, "/resources/"+videoID+"/content", nil)
	if pos := res.campo(t, "position_seconds"); pos != float64(12) {
		t.Errorf("la posición debería reemplazarse, llegó %v", pos)
	}
}

func TestLaPosicionEsDeCadaEstudiante(t *testing.T) {
	env := nuevoEntorno(t)
	videoID, cursoID := env.cursoPublicadoConVideo("prof@example.com", "curso-video", "ready")
	ana := env.inscribir("ana@example.com", cursoID)
	beto := env.inscribir("beto@example.com", cursoID)

	ana.hacer(http.MethodPut, "/resources/"+videoID+"/position", map[string]int{"position_seconds": 500})

	res := beto.hacer(http.MethodGet, "/resources/"+videoID+"/content", nil)
	if pos := res.campo(t, "position_seconds"); pos != nil && pos != float64(0) {
		t.Errorf("la posición de uno no debería afectar a otro, llegó %v", pos)
	}
}

func TestLaPosicionNoAcreditaProgreso(t *testing.T) {
	env := nuevoEntorno(t)
	videoID, cursoID := env.cursoPublicadoConVideo("prof@example.com", "curso-video", "ready")
	estudiante := env.inscribir("ana@example.com", cursoID)

	estudiante.hacer(http.MethodPut, "/resources/"+videoID+"/position", map[string]int{"position_seconds": 3600})

	// La posición la reporta el cliente: reanuda la reproducción y nada más.
	// El avance lo calcula el servidor con heartbeats y permanencia, así que
	// reportar una hora no puede dejar el recurso por completado ni sumar
	// tiempo acreditado.
	estado := env.consultarTexto(`SELECT status FROM resource_progress LIMIT 1`)
	if estado != "not_started" {
		t.Errorf("el estado no debería cambiar por reportar posición, es %q", estado)
	}
	if n := env.contar(`SELECT coalesce(sum(time_spent_seconds), 0) FROM resource_progress`); n != 0 {
		t.Errorf("no debería acreditarse tiempo, hay %d segundos", n)
	}
}

func TestSeRechazanPosicionesFueraDeRangoYTiposQueNoSeReproducen(t *testing.T) {
	env := nuevoEntorno(t)
	videoID, cursoID := env.cursoPublicadoConVideo("prof@example.com", "curso-mixto", "ready")
	pdfID := env.recursoEnCursoPublicado("curso-mixto", "pdf", "documentos/guia.pdf", "")
	estudiante := env.inscribir("ana@example.com", cursoID)

	casos := []struct {
		nombre  string
		recurso string
		valor   int
	}{
		{"negativa", videoID, -1},
		{"absurda", videoID, 999999999},
		{"un PDF no se reproduce", pdfID, 30},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			res := estudiante.hacer(http.MethodPut, "/resources/"+c.recurso+"/position",
				map[string]int{"position_seconds": c.valor})
			if res.Estado != http.StatusUnprocessableEntity {
				t.Errorf("debería rechazarse, llegó %d: %s", res.Estado, res.Crudo)
			}
		})
	}
	if n := env.contar(`SELECT count(*) FROM resource_progress`); n != 0 {
		t.Errorf("nada inválido debería persistirse, hay %d filas", n)
	}
}

func TestReportarPosicionExigeSesionYAcceso(t *testing.T) {
	env := nuevoEntorno(t)
	videoID, _ := env.cursoPublicadoConVideo("prof@example.com", "curso-video", "ready")

	if res := env.cliente().hacer(http.MethodPut, "/resources/"+videoID+"/position",
		map[string]int{"position_seconds": 10}); res.Estado != http.StatusUnauthorized {
		t.Errorf("sin sesión debería ser 401, llegó %d", res.Estado)
	}

	env.registrarYVerificar("intruso@example.com")
	intruso := env.entrar("intruso@example.com", clavePrueba)
	if res := intruso.hacer(http.MethodPut, "/resources/"+videoID+"/position",
		map[string]int{"position_seconds": 10}); res.Estado != http.StatusNotFound {
		t.Errorf("sin inscripción debería ser 404, llegó %d", res.Estado)
	}
}
