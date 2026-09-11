package httpserver_test

import (
	"context"
	"net/http"
	"testing"
)

// Estas pruebas cubren los puntos 8 y 9 del alcance mínimo: evaluación de
// selección múltiple e insignias. Se apoyan en PostgreSQL real porque lo que
// verifican (unicidad del intento, idempotencia del envío, unicidad de la
// insignia) lo garantizan restricciones de base, no el código Go.

// cursoConQuiz deja publicado un curso cuyo único recurso obligatorio es un
// quiz de dos preguntas, y devuelve el recurso y el curso.
func (e *entorno) cursoConQuiz(correoProfesor, slug string) (recursoID, cursoID string) {
	e.t.Helper()
	c, versionID := e.profesorConCurso(correoProfesor, slug)

	moduloID := moduloEnVersion(e.t, c, versionID, "Módulo 1")
	res := c.hacer(http.MethodPost, "/courses/versions/"+versionID+"/modules/"+moduloID+"/units",
		map[string]any{"title": "Unidad 1", "position": 1})
	unidadID, _ := res.campo(e.t, "ID").(string)

	// El quiz se marca obligatorio a propósito: es el único recurso del curso,
	// así que aprobarlo debe bastar para completarlo. Un curso sin ningún
	// obligatorio no acredita nada, y ese caso se cubre aparte.
	res = c.hacer(http.MethodPost, "/courses/versions/"+versionID+"/units/"+unidadID+"/resources",
		map[string]any{"type": "quiz", "title": "Evaluación final", "position": 1,
			"visible": true, "required": true})
	if res.Estado != http.StatusCreated {
		e.t.Fatalf("crear recurso quiz: %d %s", res.Estado, res.Crudo)
	}
	recursoID, _ = res.campo(e.t, "ID").(string)

	res = c.hacer(http.MethodPut,
		"/courses/versions/"+versionID+"/resources/"+recursoID+"/quiz", definicionDePrueba())
	if res.Estado != http.StatusOK {
		e.t.Fatalf("definir quiz: %d %s", res.Estado, res.Crudo)
	}

	// El 100 de recursos obligatorios es deliberado: el quiz es el único
	// obligatorio, así que aprobarlo debe bastar para completar el curso.
	res = c.hacer(http.MethodPatch, "/courses/versions/"+versionID, map[string]any{
		"title": "Curso con evaluación", "summary": "Resumen",
		"approval_min_score": 60, "approval_required_resources_pct": 100,
	})
	if res.Estado != http.StatusOK {
		e.t.Fatalf("metadatos: %d %s", res.Estado, res.Crudo)
	}
	if res := c.hacer(http.MethodPost, "/courses/versions/"+versionID+"/publish", nil); res.Estado != http.StatusOK {
		e.t.Fatalf("publicar: %d %s", res.Estado, res.Crudo)
	}
	cursoID = e.consultarTexto(`SELECT id::text FROM courses WHERE slug = $1`, slug)
	return recursoID, cursoID
}

// definicionDePrueba describe un quiz de dos preguntas, una de respuesta única
// y otra de respuesta múltiple, con 50 puntos cada una.
func definicionDePrueba() map[string]any {
	return map[string]any{
		"title": "Evaluación final", "pass_score": 60.0,
		"feedback_policy": "after_submit", "shuffle_questions": false,
		"questions": []map[string]any{
			{
				"prompt_md": "¿Cuál es la fuente de verdad transaccional?", "type": "single", "points": 1.0,
				"options": []map[string]any{
					{"text_md": "PostgreSQL", "is_correct": true},
					{"text_md": "Redis", "is_correct": false},
				},
			},
			{
				"prompt_md": "¿Qué guarda el almacenamiento de objetos?", "type": "multiple", "points": 1.0,
				"options": []map[string]any{
					{"text_md": "Los originales", "is_correct": true},
					{"text_md": "Los derivados HLS", "is_correct": true},
					{"text_md": "Las contraseñas", "is_correct": false},
				},
			},
		},
	}
}

// preguntasDelIntento extrae los stable_id de preguntas y opciones tal como se
// le presentaron al estudiante.
func preguntasDelIntento(t *testing.T, res respuesta) []map[string]any {
	t.Helper()
	crudas, ok := res.campo(t, "questions").([]any)
	if !ok {
		t.Fatalf("el intento no traía preguntas: %s", res.Crudo)
	}
	out := make([]map[string]any, 0, len(crudas))
	for _, c := range crudas {
		m, ok := c.(map[string]any)
		if !ok {
			t.Fatalf("pregunta con forma inesperada: %s", res.Crudo)
		}
		out = append(out, m)
	}
	return out
}

func opcionesDe(t *testing.T, pregunta map[string]any) []map[string]any {
	t.Helper()
	crudas, _ := pregunta["options"].([]any)
	out := make([]map[string]any, 0, len(crudas))
	for _, c := range crudas {
		m, _ := c.(map[string]any)
		out = append(out, m)
	}
	return out
}

func TestLaClaveCorrectaNuncaLlegaAlCliente(t *testing.T) {
	env := nuevoEntorno(t)
	recursoID, cursoID := env.cursoConQuiz("prof@example.com", "curso-quiz")
	estudiante := env.inscribir("ana@example.com", cursoID)

	res := estudiante.hacer(http.MethodPost, "/recursos/"+recursoID+"/quiz/intentos", nil)
	if res.Estado != http.StatusCreated {
		t.Fatalf("iniciar intento: %d %s", res.Estado, res.Crudo)
	}

	// La condición verificable "Integridad del quiz" exige que la clave no
	// salga del servidor: se comprueba sobre el JSON crudo para que ningún
	// campo añadido en el futuro la filtre sin que la prueba se entere.
	for _, prohibido := range []string{"is_correct", "isCorrect", "correct_answers", "CorrectAnswers"} {
		if contiene(res.Crudo, prohibido) {
			t.Errorf("la respuesta expone %q: %s", prohibido, res.Crudo)
		}
	}
}

func TestReiniciarNoGastaOtroIntento(t *testing.T) {
	env := nuevoEntorno(t)
	recursoID, cursoID := env.cursoConQuiz("prof@example.com", "curso-quiz")
	estudiante := env.inscribir("ana@example.com", cursoID)

	primero := estudiante.hacer(http.MethodPost, "/recursos/"+recursoID+"/quiz/intentos", nil)
	segundo := estudiante.hacer(http.MethodPost, "/recursos/"+recursoID+"/quiz/intentos", nil)

	if primero.campo(t, "attempt_id") != segundo.campo(t, "attempt_id") {
		t.Error("volver a pedir el intento abierto debería devolver el mismo, no crear otro")
	}
	if n := env.contar(`SELECT count(*) FROM quiz_attempts`); n != 1 {
		t.Errorf("se crearon %d intentos, se esperaba 1", n)
	}
}

func TestElEnvioSeCalificaEnServidorYEsIdempotente(t *testing.T) {
	env := nuevoEntorno(t)
	recursoID, cursoID := env.cursoConQuiz("prof@example.com", "curso-quiz")
	estudiante := env.inscribir("ana@example.com", cursoID)

	intento := estudiante.hacer(http.MethodPost, "/recursos/"+recursoID+"/quiz/intentos", nil)
	attemptID, _ := intento.campo(t, "attempt_id").(string)

	// Se responde bien solo la primera pregunta: la nota debe ser 50 y no
	// alcanzar el 60 exigido para aprobar.
	preguntas := preguntasDelIntento(t, intento)
	primera := preguntas[0]
	correcta := ""
	for _, o := range opcionesDe(t, primera) {
		if o["text_md"] == "PostgreSQL" {
			correcta, _ = o["stable_id"].(string)
		}
	}
	res := estudiante.hacer(http.MethodPut, "/quiz/intentos/"+attemptID+"/respuestas", map[string]any{
		"question_stable_id":         primera["stable_id"],
		"selected_option_stable_ids": []string{correcta},
	})
	if res.Estado != http.StatusOK {
		t.Fatalf("guardado parcial: %d %s", res.Estado, res.Crudo)
	}

	envio := estudiante.hacer(http.MethodPost, "/quiz/intentos/"+attemptID+"/enviar", nil,
		[2]string{"Idempotency-Key", "clave-de-envio-1"})
	if envio.Estado != http.StatusOK {
		t.Fatalf("envío: %d %s", envio.Estado, envio.Crudo)
	}
	nota, _ := envio.campo(t, "score").(float64)
	if nota != 50 {
		t.Errorf("nota %v, se esperaba 50 (una de dos preguntas)", nota)
	}
	if aprobado, _ := envio.campo(t, "passed").(bool); aprobado {
		t.Error("50 no debería aprobar con un mínimo de 60")
	}

	// Reintentar con la misma clave devuelve la nota guardada en vez de
	// calificar otra vez.
	repetido := estudiante.hacer(http.MethodPost, "/quiz/intentos/"+attemptID+"/enviar", nil,
		[2]string{"Idempotency-Key", "clave-de-envio-1"})
	if repetido.Estado != http.StatusOK {
		t.Fatalf("reenvío: %d %s", repetido.Estado, repetido.Crudo)
	}
	if repetido.campo(t, "score") != envio.campo(t, "score") {
		t.Errorf("el reenvío cambió la nota: %v contra %v",
			repetido.campo(t, "score"), envio.campo(t, "score"))
	}
	if n := env.contar(`SELECT count(*) FROM quiz_attempts WHERE status='submitted'`); n != 1 {
		t.Errorf("hay %d intentos calificados, se esperaba 1", n)
	}
}

func TestNoSePuedeResponderConOpcionesDeOtroQuiz(t *testing.T) {
	env := nuevoEntorno(t)
	recursoID, cursoID := env.cursoConQuiz("prof@example.com", "curso-quiz")
	estudiante := env.inscribir("ana@example.com", cursoID)

	intento := estudiante.hacer(http.MethodPost, "/recursos/"+recursoID+"/quiz/intentos", nil)
	attemptID, _ := intento.campo(t, "attempt_id").(string)
	preguntas := preguntasDelIntento(t, intento)

	res := estudiante.hacer(http.MethodPut, "/quiz/intentos/"+attemptID+"/respuestas", map[string]any{
		"question_stable_id":         preguntas[0]["stable_id"],
		"selected_option_stable_ids": []string{"11111111-1111-1111-1111-111111111111"},
	})
	if res.Estado != http.StatusUnprocessableEntity {
		t.Errorf("una opción ajena al intento debería rechazarse, se obtuvo %d %s", res.Estado, res.Crudo)
	}
}

func TestElIntentoAjenoNoSeRevela(t *testing.T) {
	env := nuevoEntorno(t)
	recursoID, cursoID := env.cursoConQuiz("prof@example.com", "curso-quiz")
	ana := env.inscribir("ana@example.com", cursoID)
	beto := env.inscribir("beto@example.com", cursoID)

	intento := ana.hacer(http.MethodPost, "/recursos/"+recursoID+"/quiz/intentos", nil)
	attemptID, _ := intento.campo(t, "attempt_id").(string)

	// Se responde 404 y no 403: confirmar que el intento existe ya sería
	// filtrar información de otra estudiante.
	if res := beto.hacer(http.MethodGet, "/quiz/intentos/"+attemptID, nil); res.Estado != http.StatusNotFound {
		t.Errorf("el intento ajeno debería responder 404, se obtuvo %d %s", res.Estado, res.Crudo)
	}
}

func TestAprobarElQuizEmiteUnaInsigniaUnicaYVerificable(t *testing.T) {
	env := nuevoEntorno(t)
	recursoID, cursoID := env.cursoConQuiz("prof@example.com", "curso-quiz")
	estudiante := env.inscribir("ana@example.com", cursoID)

	// El quiz es el único recurso obligatorio, así que completarlo y aprobarlo
	// debe bastar para aprobar el curso y emitir la insignia.
	intento := estudiante.hacer(http.MethodPost, "/recursos/"+recursoID+"/quiz/intentos", nil)
	attemptID, _ := intento.campo(t, "attempt_id").(string)
	responderTodoBien(t, estudiante, attemptID, preguntasDelIntento(t, intento))

	if res := estudiante.hacer(http.MethodPost, "/recursos/"+recursoID+"/progreso",
		map[string]any{"type": "close", "complete": true}); res.Estado != http.StatusOK {
		t.Fatalf("progreso: %d %s", res.Estado, res.Crudo)
	}

	envio := estudiante.hacer(http.MethodPost, "/quiz/intentos/"+attemptID+"/enviar", nil)
	if envio.Estado != http.StatusOK {
		t.Fatalf("envío: %d %s", envio.Estado, envio.Crudo)
	}
	if aprobado, _ := envio.campo(t, "passed").(bool); !aprobado {
		t.Fatalf("responder todo bien debería aprobar: %s", envio.Crudo)
	}

	resumen := estudiante.hacer(http.MethodGet, "/cursos/"+cursoID+"/progreso", nil)
	if estado, _ := resumen.campo(t, "status").(string); estado != "approved" {
		t.Fatalf("el curso debería quedar approved, quedó %q: %s", estado, resumen.Crudo)
	}
	codigo, _ := resumen.campo(t, "badge_code").(string)
	if codigo == "" {
		t.Fatalf("no se emitió la insignia: %s", resumen.Crudo)
	}
	if n := env.contar(`SELECT count(*) FROM badges`); n != 1 {
		t.Errorf("se emitieron %d insignias, la emisión debe ser única", n)
	}

	// La verificación es pública y no puede exponer el correo del estudiante.
	publico := env.cliente().hacer(http.MethodGet, "/insignias/"+codigo, nil)
	if publico.Estado != http.StatusOK {
		t.Fatalf("verificación pública: %d %s", publico.Estado, publico.Crudo)
	}
	if valida, _ := publico.campo(t, "valid").(bool); !valida {
		t.Error("la insignia recién emitida debería ser válida")
	}
	if contiene(publico.Crudo, "ana@example.com") || contiene(publico.Crudo, "student_id") {
		t.Errorf("la verificación pública expone datos personales: %s", publico.Crudo)
	}

	// El alcance mínimo pide insignia "con imagen y URL verificable". La
	// imagen se genera al emitir y vive en el almacén de objetos, no en
	// Postgres.
	urlImagen, _ := publico.campo(t, "image_url").(string)
	if urlImagen == "" {
		t.Fatalf("la verificación no trae la imagen de la insignia: %s", publico.Crudo)
	}
	svg, ok := svgDeInsigniaGuardado("badges/" + codigo + ".svg")
	if !ok {
		t.Fatalf("no se guardó ninguna imagen para la insignia %s", codigo)
	}
	if !contiene(svg, "<svg") {
		t.Errorf("lo guardado no parece un SVG: %.120s", svg)
	}
	// La imagen se comparte, así que tampoco puede llevar datos personales.
	if contiene(svg, "ana@example.com") || contiene(svg, "Ana") {
		t.Errorf("la imagen de la insignia expone datos personales: %s", svg)
	}
	if !contiene(svg, codigo) {
		t.Error("la imagen no lleva el código de verificación, así que no se puede comprobar")
	}
	if n := env.contar(`SELECT count(*) FROM badges WHERE image_object_key IS NOT NULL AND image_object_key <> ''`); n != 1 {
		t.Errorf("la insignia quedó sin clave de imagen en la base")
	}

	// Y el estudiante la ve en su listado, con el título del curso resuelto.
	mias := estudiante.hacer(http.MethodGet, "/insignias/mias", nil)
	if mias.Estado != http.StatusOK {
		t.Fatalf("mis insignias: %d %s", mias.Estado, mias.Crudo)
	}
	if !contiene(mias.Crudo, codigo) || !contiene(mias.Crudo, "course_title") {
		t.Errorf("el listado propio no trae la insignia con su curso: %s", mias.Crudo)
	}
}

func TestUnCursoSinObligatoriosNoRepartInsigniasGratis(t *testing.T) {
	env := nuevoEntorno(t)
	recursoID, cursoID := env.cursoConQuiz("prof@example.com", "curso-quiz")

	// Se deja el curso sin ningún recurso obligatorio. El cálculo de avance
	// sobre una lista vacía da 100 por ciento, así que sin una guarda explícita
	// bastaría con inscribirse para recibir una insignia verificable.
	if _, err := env.pool.Exec(context.Background(),
		`UPDATE resources SET required = false WHERE id = $1`, recursoID); err != nil {
		t.Fatalf("marcar el recurso como opcional: %v", err)
	}

	estudiante := env.inscribir("ana@example.com", cursoID)
	if res := estudiante.hacer(http.MethodPost, "/recursos/"+recursoID+"/progreso",
		map[string]any{"type": "close", "complete": true}); res.Estado != http.StatusOK {
		t.Fatalf("progreso: %d %s", res.Estado, res.Crudo)
	}

	resumen := estudiante.hacer(http.MethodGet, "/cursos/"+cursoID+"/progreso", nil)
	if estado, _ := resumen.campo(t, "status").(string); estado == "approved" {
		t.Errorf("un curso sin obligatorios no debería aprobar a nadie: %s", resumen.Crudo)
	}
	if n := env.contar(`SELECT count(*) FROM badges`); n != 0 {
		t.Errorf("se emitieron %d insignias sin haber nada que completar", n)
	}
}

func TestElHeartbeatImplausibleSeRechazaYSeAudita(t *testing.T) {
	env := nuevoEntorno(t)
	recursoID, cursoID := env.cursoConQuiz("prof@example.com", "curso-quiz")
	estudiante := env.inscribir("ana@example.com", cursoID)

	// Dos heartbeats seguidos: el segundo llega mucho antes del intervalo
	// mínimo, así que no debe acreditar tiempo.
	estudiante.hacer(http.MethodPost, "/recursos/"+recursoID+"/progreso", map[string]any{"type": "open"})
	if res := estudiante.hacer(http.MethodPost, "/recursos/"+recursoID+"/progreso",
		map[string]any{"type": "heartbeat"}); res.Estado != http.StatusOK {
		t.Fatalf("heartbeat: %d %s", res.Estado, res.Crudo)
	}

	acreditado := env.contar(`
		SELECT coalesce(sum(duration_seconds), 0) FROM progress_events WHERE event_type='heartbeat'`)
	if acreditado != 0 {
		t.Errorf("se acreditaron %d segundos por un heartbeat implausible, se esperaba 0", acreditado)
	}
	if n := env.contar(`SELECT count(*) FROM audit_logs WHERE action='progress.rejected'`); n != 1 {
		t.Errorf("el rechazo debería auditarse, hay %d entradas", n)
	}
}

func TestElProgresoNoAceptaPorcentajesDelCliente(t *testing.T) {
	env := nuevoEntorno(t)
	recursoID, cursoID := env.cursoConQuiz("prof@example.com", "curso-quiz")
	estudiante := env.inscribir("ana@example.com", cursoID)

	// El cliente intenta declararse completo con un tipo de evento inventado.
	res := estudiante.hacer(http.MethodPost, "/recursos/"+recursoID+"/progreso",
		map[string]any{"type": "percent", "complete": true})
	if res.Estado != http.StatusUnprocessableEntity {
		t.Errorf("un tipo de evento inventado debería rechazarse, se obtuvo %d %s", res.Estado, res.Crudo)
	}
	if n := env.contar(`SELECT count(*) FROM resource_progress WHERE status='completed'`); n != 0 {
		t.Error("un evento rechazado no debería completar el recurso")
	}
}

// responderTodoBien marca todas las opciones correctas de todas las preguntas.
//
// Consulta la clave directamente en base porque, por diseño, la API nunca se
// la entrega al cliente: esa es justamente la propiedad que se está probando
// en el resto del archivo.
func responderTodoBien(t *testing.T, c *cliente, attemptID string, preguntas []map[string]any) {
	t.Helper()
	for _, p := range preguntas {
		stableID, _ := p["stable_id"].(string)
		var correctas []string
		filas, err := c.env.pool.Query(context.Background(), `
			SELECT o.stable_id::text FROM quiz_options o
			  JOIN quiz_questions q ON q.id = o.question_id
			 WHERE q.stable_id = $1 AND o.is_correct`, stableID)
		if err != nil {
			t.Fatalf("clave correcta: %v", err)
		}
		for filas.Next() {
			var id string
			if err := filas.Scan(&id); err != nil {
				filas.Close()
				t.Fatalf("clave correcta: %v", err)
			}
			correctas = append(correctas, id)
		}
		filas.Close()

		res := c.hacer(http.MethodPut, "/quiz/intentos/"+attemptID+"/respuestas", map[string]any{
			"question_stable_id":         stableID,
			"selected_option_stable_ids": correctas,
		})
		if res.Estado != http.StatusOK {
			t.Fatalf("guardar respuesta: %d %s", res.Estado, res.Crudo)
		}
	}
}
