package httpserver_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/google/uuid"
)

// cursoPublicadoConVideo deja un curso publicado con un recurso de video, y
// devuelve el identificador del recurso y el del curso.
func (e *entorno) cursoPublicadoConVideo(correoProfesor, slug, estadoMedia string) (recursoID, cursoID string) {
	e.t.Helper()
	c, versionID := e.profesorConCurso(correoProfesor, slug)

	moduloID := moduloEnVersion(e.t, c, versionID, "Módulo 1")
	res := c.hacer(http.MethodPost, "/courses/versions/"+versionID+"/modules/"+moduloID+"/units",
		map[string]any{"title": "Unidad 1", "position": 1})
	unidadID, _ := res.campo(e.t, "ID").(string)

	res = c.hacer(http.MethodPost, "/courses/versions/"+versionID+"/units/"+unidadID+"/resources",
		map[string]any{"type": "video", "title": "Clase 1", "position": 1, "visible": true})
	if res.Estado != http.StatusCreated {
		e.t.Fatalf("crear recurso: %d %s", res.Estado, res.Crudo)
	}
	recursoID, _ = res.campo(e.t, "ID").(string)

	// El recurso debe estar listo para poder publicar la versión.
	ctx := context.Background()
	if _, err := e.pool.Exec(ctx,
		`UPDATE resources SET processing_status='ready' WHERE id=$1`, recursoID); err != nil {
		e.t.Fatalf("estado del recurso: %v", err)
	}
	if _, err := e.pool.Exec(ctx, `
		INSERT INTO media_assets (id, resource_id, original_object_key, status, hls_master_key)
		VALUES ($1, $2, 'originales/clase1.mp4', $3, $4)`,
		uuid.New(), recursoID, estadoMedia, claveHLS(estadoMedia, recursoID)); err != nil {
		e.t.Fatalf("activo multimedia: %v", err)
	}

	res = c.hacer(http.MethodPatch, "/courses/versions/"+versionID, map[string]any{
		"title": "Curso publicable", "summary": "Resumen",
		"approval_min_score": 60, "approval_required_resources_pct": 80,
	})
	if res.Estado != http.StatusOK {
		e.t.Fatalf("metadatos: %d %s", res.Estado, res.Crudo)
	}
	if res := c.hacer(http.MethodPost, "/courses/versions/"+versionID+"/publish", nil); res.Estado != http.StatusOK {
		e.t.Fatalf("publicar: %d %s", res.Estado, res.Crudo)
	}

	cursoID = e.consultarTexto(`SELECT id::text FROM courses WHERE slug=$1`, slug)
	return recursoID, cursoID
}

func claveHLS(estado, recursoID string) string {
	if estado != "ready" {
		return ""
	}
	return "hls/" + recursoID + "/master.m3u8"
}

// inscribir deja a un estudiante inscrito y devuelve su cliente.
func (e *entorno) inscribir(correo, cursoID string) *cliente {
	e.t.Helper()
	e.registrarYVerificar(correo)
	c := e.entrar(correo, clavePrueba)
	if res := c.hacer(http.MethodPost, "/enrollments", map[string]string{"course_id": cursoID}); res.Estado != http.StatusCreated {
		e.t.Fatalf("inscripción: %d %s", res.Estado, res.Crudo)
	}
	return c
}

func TestElEstudianteInscritoObtieneLaURLDelHLS(t *testing.T) {
	env := nuevoEntorno(t)
	recursoID, cursoID := env.cursoPublicadoConVideo("prof@example.com", "curso-video", "ready")
	estudiante := env.inscribir("ana@example.com", cursoID)

	res := estudiante.hacer(http.MethodGet, "/resources/"+recursoID+"/content", nil)
	if res.Estado != http.StatusOK {
		t.Fatalf("%d %s", res.Estado, res.Crudo)
	}
	url, _ := res.campo(t, "url").(string)
	if url != "https://cdn.pruebas.local/hls/"+recursoID+"/master.m3u8" {
		t.Errorf("URL inesperada: %q", url)
	}
	if tipo := res.campo(t, "type"); tipo != "video" {
		t.Errorf("el tipo debería ser el del recurso, llegó %v", tipo)
	}
	if res.campo(t, "cdn") != true {
		t.Error("con base pública configurada, la entrega debería marcarse como de CDN")
	}
}

func TestQuienNoEstaInscritoNoObtieneElHLS(t *testing.T) {
	env := nuevoEntorno(t)
	recursoID, _ := env.cursoPublicadoConVideo("prof@example.com", "curso-video", "ready")

	env.registrarYVerificar("intruso@example.com")
	intruso := env.entrar("intruso@example.com", clavePrueba)

	if res := intruso.hacer(http.MethodGet, "/resources/"+recursoID+"/content", nil); res.Estado != http.StatusNotFound {
		t.Errorf("sin inscripción no debería entregarse, llegó %d: %s", res.Estado, res.Crudo)
	}
	if res := env.cliente().hacer(http.MethodGet, "/resources/"+recursoID+"/content", nil); res.Estado != http.StatusUnauthorized {
		t.Errorf("sin sesión debería ser 401, llegó %d", res.Estado)
	}
}

func TestRetirarseCortaElAccesoAlContenido(t *testing.T) {
	env := nuevoEntorno(t)
	recursoID, cursoID := env.cursoPublicadoConVideo("prof@example.com", "curso-video", "ready")
	estudiante := env.inscribir("ana@example.com", cursoID)

	if res := estudiante.hacer(http.MethodGet, "/resources/"+recursoID+"/content", nil); res.Estado != http.StatusOK {
		t.Fatalf("antes de retirarse debería funcionar: %d %s", res.Estado, res.Crudo)
	}
	if res := estudiante.hacer(http.MethodPost, "/enrollments/"+cursoID+"/withdraw", nil); res.Estado != http.StatusOK {
		t.Fatalf("retiro: %d %s", res.Estado, res.Crudo)
	}
	if res := estudiante.hacer(http.MethodGet, "/resources/"+recursoID+"/content", nil); res.Estado != http.StatusNotFound {
		t.Errorf("tras retirarse no debería entregarse, llegó %d: %s", res.Estado, res.Crudo)
	}
}

func TestElProfesorDuenoAccedeSinInscribirse(t *testing.T) {
	env := nuevoEntorno(t)
	recursoID, _ := env.cursoPublicadoConVideo("prof@example.com", "curso-video", "ready")
	profesor := env.entrar("prof@example.com", clavePrueba)

	if res := profesor.hacer(http.MethodGet, "/resources/"+recursoID+"/content", nil); res.Estado != http.StatusOK {
		t.Errorf("el profesor dueño debería poder revisar su propio material, llegó %d: %s", res.Estado, res.Crudo)
	}
}

func TestUnRecursoSinTranscodificarNoSeEntrega(t *testing.T) {
	env := nuevoEntorno(t)
	recursoID, cursoID := env.cursoPublicadoConVideo("prof@example.com", "curso-video", "processing")
	estudiante := env.inscribir("ana@example.com", cursoID)

	res := estudiante.hacer(http.MethodGet, "/resources/"+recursoID+"/content", nil)
	if res.Estado != http.StatusConflict {
		t.Fatalf("%d %s", res.Estado, res.Crudo)
	}
	if codigo := res.campo(t, "error", "code"); codigo != "media_not_ready" {
		t.Errorf("código inesperado: %v", codigo)
	}
}

func TestUnRecursoOcultoNoSeEntregaAunqueEsteListo(t *testing.T) {
	env := nuevoEntorno(t)
	recursoID, cursoID := env.cursoPublicadoConVideo("prof@example.com", "curso-video", "ready")
	estudiante := env.inscribir("ana@example.com", cursoID)

	if _, err := env.pool.Exec(context.Background(),
		`UPDATE resources SET visible=false WHERE id=$1`, recursoID); err != nil {
		t.Fatalf("ocultar el recurso: %v", err)
	}

	// Un recurso oculto responde como inexistente: confirmar que existe ya
	// delataría contenido que el autor no ha querido mostrar.
	if res := estudiante.hacer(http.MethodGet, "/resources/"+recursoID+"/content", nil); res.Estado != http.StatusNotFound {
		t.Errorf("un recurso oculto no debería entregarse, llegó %d: %s", res.Estado, res.Crudo)
	}
}

func TestUnRecursoDeUnBorradorNoSeEntrega(t *testing.T) {
	env := nuevoEntorno(t)
	recursoID, cursoID := env.cursoPublicadoConVideo("prof@example.com", "curso-video", "ready")
	estudiante := env.inscribir("ana@example.com", cursoID)

	// Se despublica el curso: el recurso deja de pertenecer a una versión
	// vigente y no debe seguir sirviéndose.
	profesor := env.entrar("prof@example.com", clavePrueba)
	if res := profesor.hacer(http.MethodPost, "/courses/"+cursoID+"/unpublish", nil); res.Estado != http.StatusOK {
		t.Fatalf("despublicar: %d %s", res.Estado, res.Crudo)
	}
	if res := estudiante.hacer(http.MethodGet, "/resources/"+recursoID+"/content", nil); res.Estado != http.StatusNotFound {
		t.Errorf("un recurso despublicado no debería entregarse, llegó %d: %s", res.Estado, res.Crudo)
	}
}
