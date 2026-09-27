package httpserver_test

import (
	"context"
	"net/http"
	"testing"
)

// Cubren el alcance opcional 5.2: panel administrativo con métricas y
// resultados agregados por quiz.

// responderMal elige a propósito una opción incorrecta de cada pregunta, para
// que el informe tenga una distribución que mirar y no solo aciertos.
func responderMal(t *testing.T, c *cliente, attemptID string, preguntas []map[string]any) {
	t.Helper()
	for _, p := range preguntas {
		stableID, _ := p["stable_id"].(string)
		var incorrecta string
		if err := c.env.pool.QueryRow(context.Background(), `
			SELECT o.stable_id::text FROM quiz_options o
			  JOIN quiz_questions q ON q.id = o.question_id
			 WHERE q.stable_id = $1 AND NOT o.is_correct
			 ORDER BY o.position LIMIT 1`, stableID).Scan(&incorrecta); err != nil {
			t.Fatalf("opción incorrecta: %v", err)
		}
		res := c.hacer(http.MethodPut, "/quiz/attempts/"+attemptID+"/answers", map[string]any{
			"question_stable_id":         stableID,
			"selected_option_stable_ids": []string{incorrecta},
		})
		if res.Estado != http.StatusOK {
			t.Fatalf("guardar respuesta: %d %s", res.Estado, res.Crudo)
		}
	}
}

func TestSoloLaAdministracionVeLasMetricas(t *testing.T) {
	env := nuevoEntorno(t)
	c, _ := env.profesorConCurso("prof@example.com", "curso-metricas")

	if res := c.hacer(http.MethodGet, "/admin/metrics", nil); res.Estado != http.StatusForbidden {
		t.Errorf("un profesor no debería ver las métricas de la plataforma, llegó %d", res.Estado)
	}
}

func TestLasMetricasDesglosanElEstadoDeLaPlataforma(t *testing.T) {
	env := nuevoEntorno(t)
	admin := env.administrador("admin@example.com")
	c, cursoID, _ := env.cursoPublicadoConLectura("prof@example.com", "curso-metricas")
	_ = c
	env.inscribir("ana@example.com", cursoID)

	res := admin.hacer(http.MethodGet, "/admin/metrics", nil)
	if res.Estado != http.StatusOK {
		t.Fatalf("métricas: %d %s", res.Estado, res.Crudo)
	}

	usuarios, _ := res.campo(t, "usuarios").(map[string]any)
	if total, _ := usuarios["total"].(float64); total < 3 {
		t.Errorf("se esperaban al menos tres cuentas (admin, profesor, estudiante): %s", res.Crudo)
	}
	desglose, _ := usuarios["desglose"].(map[string]any)
	if _, hay := desglose["teacher/active"]; !hay {
		t.Errorf("el desglose no distingue rol y estado: %s", res.Crudo)
	}

	cursos, _ := res.campo(t, "cursos").(map[string]any)
	cursosDesglose, _ := cursos["desglose"].(map[string]any)
	if n, _ := cursosDesglose["published"].(float64); n != 1 {
		t.Errorf("debería constar una versión publicada: %s", res.Crudo)
	}

	inscripciones, _ := res.campo(t, "inscripciones").(map[string]any)
	if total, _ := inscripciones["total"].(float64); total != 1 {
		t.Errorf("debería constar una inscripción: %s", res.Crudo)
	}
}

// El informe de una evaluación es del profesor que la escribió, que es quien
// puede corregir una pregunta mal planteada.
func TestElProfesorVeLosResultadosAgregadosDeSuQuiz(t *testing.T) {
	env := nuevoEntorno(t)
	recursoID, cursoID := env.cursoConQuiz("prof@example.com", "curso-quiz")
	versionID := env.consultarTexto(
		`SELECT current_published_version_id::text FROM courses WHERE id = $1`, cursoID)
	profesor := env.entrar("prof@example.com", clavePrueba)

	// Dos estudiantes: una acierta todo, la otra falla la primera pregunta.
	ana := env.inscribir("ana@example.com", cursoID)
	intentoAna := ana.hacer(http.MethodPost, "/resources/"+recursoID+"/quiz/attempts", nil)
	idAna, _ := intentoAna.campo(t, "attempt_id").(string)
	responderTodoBien(t, ana, idAna, preguntasDelIntento(t, intentoAna))
	ana.hacer(http.MethodPost, "/quiz/attempts/"+idAna+"/submit", nil)

	luis := env.inscribir("luis@example.com", cursoID)
	intentoLuis := luis.hacer(http.MethodPost, "/resources/"+recursoID+"/quiz/attempts", nil)
	idLuis, _ := intentoLuis.campo(t, "attempt_id").(string)
	responderMal(t, luis, idLuis, preguntasDelIntento(t, intentoLuis))
	luis.hacer(http.MethodPost, "/quiz/attempts/"+idLuis+"/submit", nil)

	res := profesor.hacer(http.MethodGet,
		"/courses/versions/"+versionID+"/resources/"+recursoID+"/results", nil)
	if res.Estado != http.StatusOK {
		t.Fatalf("resultados: %d %s", res.Estado, res.Crudo)
	}

	resultados, _ := res.campo(t, "resultados").(map[string]any)
	if intentos, _ := resultados["intentos"].(float64); intentos != 2 {
		t.Errorf("se esperaban 2 intentos: %s", res.Crudo)
	}
	if estudiantes, _ := resultados["estudiantes"].(float64); estudiantes != 2 {
		t.Errorf("se esperaban 2 estudiantes: %s", res.Crudo)
	}

	preguntas, _ := resultados["preguntas"].([]any)
	if len(preguntas) != 2 {
		t.Fatalf("se esperaban 2 preguntas en el informe: %s", res.Crudo)
	}
	primera, _ := preguntas[0].(map[string]any)
	if primera["prompt_md"] == "" || primera["prompt_md"] == nil {
		t.Errorf("el informe no trae el enunciado: %s", res.Crudo)
	}
	opciones, _ := primera["opciones"].([]any)
	if len(opciones) == 0 {
		t.Fatalf("el informe no trae la distribución por opción: %s", res.Crudo)
	}
	// El informe sí revela cuál era la correcta: lo ve su autor, y sin eso no
	// serviría para detectar un distractor ambiguo.
	var hayCorrecta bool
	for _, o := range opciones {
		m, _ := o.(map[string]any)
		if correcta, _ := m["es_correcta"].(bool); correcta {
			hayCorrecta = true
		}
	}
	if !hayCorrecta {
		t.Errorf("el informe no señala la opción correcta: %s", res.Crudo)
	}
}

// Un profesor ajeno no ve los resultados de un quiz que no es suyo.
func TestUnProfesorAjenoNoVeLosResultados(t *testing.T) {
	env := nuevoEntorno(t)
	recursoID, cursoID := env.cursoConQuiz("prof@example.com", "curso-quiz")
	versionID := env.consultarTexto(
		`SELECT current_published_version_id::text FROM courses WHERE id = $1`, cursoID)

	env.registrarYVerificar("otro@example.com")
	env.asciendeA("otro@example.com", "teacher")
	otro := env.entrar("otro@example.com", clavePrueba)

	res := otro.hacer(http.MethodGet,
		"/courses/versions/"+versionID+"/resources/"+recursoID+"/results", nil)
	if res.Estado == http.StatusOK {
		t.Errorf("un profesor ajeno no debería ver los resultados, llegó %d: %s", res.Estado, res.Crudo)
	}
}

// Sin intentos el informe existe y viene vacío: no es un error, es un quiz que
// todavía no ha presentado nadie.
func TestUnQuizSinIntentosDevuelveUnInformeVacio(t *testing.T) {
	env := nuevoEntorno(t)
	recursoID, cursoID := env.cursoConQuiz("prof@example.com", "curso-quiz")
	versionID := env.consultarTexto(
		`SELECT current_published_version_id::text FROM courses WHERE id = $1`, cursoID)
	profesor := env.entrar("prof@example.com", clavePrueba)

	res := profesor.hacer(http.MethodGet,
		"/courses/versions/"+versionID+"/resources/"+recursoID+"/results", nil)
	if res.Estado != http.StatusOK {
		t.Fatalf("resultados: %d %s", res.Estado, res.Crudo)
	}
	resultados, _ := res.campo(t, "resultados").(map[string]any)
	if intentos, _ := resultados["intentos"].(float64); intentos != 0 {
		t.Errorf("se esperaban 0 intentos: %s", res.Crudo)
	}
	if preguntas, _ := resultados["preguntas"].([]any); preguntas == nil {
		t.Errorf("las preguntas deberían venir como lista vacía, no como null: %s", res.Crudo)
	}
}
