package httpserver_test

import (
	"net/http"
	"testing"
)

// Cubren dos partes del alcance opcional 5.2: coautoría básica y exportación
// de datos personales.

// otroProfesor deja lista una segunda cuenta de profesor.
func (e *entorno) otroProfesor(correo string) *cliente {
	e.t.Helper()
	e.registrarYVerificar(correo)
	e.asciendeA(correo, "teacher")
	return e.entrar(correo, clavePrueba)
}

func TestUnColaboradorPuedeEditarElCursoAjeno(t *testing.T) {
	env := nuevoEntorno(t)
	dueno, versionID := env.profesorConCurso("dueno@example.com", "curso-coautoria")
	cursoID := env.consultarTexto(`SELECT id::text FROM courses WHERE slug = 'curso-coautoria'`)
	otro := env.otroProfesor("colega@example.com")

	// Antes de invitarlo, no puede ni mirar el borrador.
	if res := otro.hacer(http.MethodGet, "/courses/versions/"+versionID, nil); res.Estado == http.StatusOK {
		t.Fatalf("un profesor ajeno no debería ver el borrador, llegó %d", res.Estado)
	}

	if res := dueno.hacer(http.MethodPost, "/collaborators/"+cursoID,
		map[string]any{"email": "colega@example.com"}); res.Estado != http.StatusCreated {
		t.Fatalf("invitar colaborador: %d %s", res.Estado, res.Crudo)
	}

	// Ahora sí: ve el borrador y puede añadir estructura.
	if res := otro.hacer(http.MethodGet, "/courses/versions/"+versionID, nil); res.Estado != http.StatusOK {
		t.Errorf("el colaborador debería ver el borrador, llegó %d: %s", res.Estado, res.Crudo)
	}
	if res := otro.hacer(http.MethodPost, "/courses/versions/"+versionID+"/modules",
		map[string]any{"title": "Módulo del colega", "position": 1}); res.Estado != http.StatusCreated {
		t.Errorf("el colaborador debería poder editar, llegó %d: %s", res.Estado, res.Crudo)
	}
}

// La línea que separa la coautoría básica de una gestión de permisos: un
// colaborador edita, pero no reparte acceso. Si pudiera, invitar a alguien
// equivaldría a cederle el curso.
func TestUnColaboradorNoPuedeInvitarAOtros(t *testing.T) {
	env := nuevoEntorno(t)
	dueno, _ := env.profesorConCurso("dueno@example.com", "curso-coautoria")
	cursoID := env.consultarTexto(`SELECT id::text FROM courses WHERE slug = 'curso-coautoria'`)
	colega := env.otroProfesor("colega@example.com")
	env.otroProfesor("tercero@example.com")

	dueno.hacer(http.MethodPost, "/collaborators/"+cursoID,
		map[string]any{"email": "colega@example.com"})

	res := colega.hacer(http.MethodPost, "/collaborators/"+cursoID,
		map[string]any{"email": "tercero@example.com"})
	if res.Estado != http.StatusForbidden {
		t.Errorf("un colaborador no debería repartir acceso, llegó %d: %s", res.Estado, res.Crudo)
	}
	// Tampoco ver la lista, que es parte de la administración del curso.
	if lista := colega.hacer(http.MethodGet, "/collaborators/"+cursoID, nil); lista.Estado == http.StatusOK {
		t.Errorf("un colaborador no debería ver la lista, llegó %d", lista.Estado)
	}
}

func TestQuitarUnColaboradorLeCortaElAcceso(t *testing.T) {
	env := nuevoEntorno(t)
	dueno, versionID := env.profesorConCurso("dueno@example.com", "curso-coautoria")
	cursoID := env.consultarTexto(`SELECT id::text FROM courses WHERE slug = 'curso-coautoria'`)
	colega := env.otroProfesor("colega@example.com")

	dueno.hacer(http.MethodPost, "/collaborators/"+cursoID,
		map[string]any{"email": "colega@example.com"})

	lista := dueno.hacer(http.MethodGet, "/collaborators/"+cursoID, nil)
	items, _ := lista.campo(t, "items").([]any)
	if len(items) != 1 {
		t.Fatalf("se esperaba un colaborador: %s", lista.Crudo)
	}
	entrada, _ := items[0].(map[string]any)
	userID, _ := entrada["user_id"].(string)

	if res := dueno.hacer(http.MethodDelete, "/collaborators/"+cursoID+"/"+userID, nil); res.Estado != http.StatusNoContent {
		t.Fatalf("quitar colaborador: %d %s", res.Estado, res.Crudo)
	}
	if res := colega.hacer(http.MethodGet, "/courses/versions/"+versionID, nil); res.Estado == http.StatusOK {
		t.Errorf("tras quitarlo no debería seguir viendo el borrador, llegó %d", res.Estado)
	}
}

// Invitar por correo no puede servir para averiguar qué correos existen: la
// respuesta es la misma exista o no la cuenta.
func TestInvitarNoRevelaQueCorreosExisten(t *testing.T) {
	env := nuevoEntorno(t)
	dueno, _ := env.profesorConCurso("dueno@example.com", "curso-coautoria")
	cursoID := env.consultarTexto(`SELECT id::text FROM courses WHERE slug = 'curso-coautoria'`)

	// Una cuenta que existe pero es de estudiante.
	env.registrarYVerificar("estudiante@example.com")

	inexistente := dueno.hacer(http.MethodPost, "/collaborators/"+cursoID,
		map[string]any{"email": "nadie@example.com"})
	estudiante := dueno.hacer(http.MethodPost, "/collaborators/"+cursoID,
		map[string]any{"email": "estudiante@example.com"})

	if inexistente.Estado != estudiante.Estado || inexistente.Crudo != estudiante.Crudo {
		t.Errorf("las respuestas distinguen una cuenta inexistente de una que no es profesor:\n%s\n%s",
			inexistente.Crudo, estudiante.Crudo)
	}
	if inexistente.Estado == http.StatusCreated {
		t.Errorf("no debería haberse concedido acceso: %s", inexistente.Crudo)
	}
}

// --- Exportación de datos personales ---

func TestLaExportacionEntregaLosDatosDeQuienLaPide(t *testing.T) {
	env := nuevoEntorno(t)
	_, cursoID, recursoID := env.cursoPublicadoConLectura("prof@example.com", "curso-export")
	estudiante := env.inscribir("ana@example.com", cursoID)

	estudiante.hacer(http.MethodPost, "/resources/"+recursoID+"/progress", map[string]any{"type": "open"})

	res := estudiante.hacer(http.MethodGet, "/me/export", nil)
	if res.Estado != http.StatusOK {
		t.Fatalf("exportar: %d %s", res.Estado, res.Crudo)
	}

	cuenta, _ := res.campo(t, "cuenta").(map[string]any)
	if correo, _ := cuenta["email"].(string); correo != "ana@example.com" {
		t.Errorf("la exportación no es de quien la pidió: %s", res.Crudo)
	}
	if inscripciones, _ := res.campo(t, "inscripciones").([]any); len(inscripciones) != 1 {
		t.Errorf("debería incluir su inscripción: %s", res.Crudo)
	}
	if !contiene(res.Crudo, "progreso") {
		t.Errorf("debería incluir su progreso: %s", res.Crudo)
	}

	// Se entrega como descarga y sin quedar en ninguna caché.
	if disp := res.Cabecera.Get("Content-Disposition"); !contiene(disp, "attachment") {
		t.Errorf("debería entregarse como descarga, llegó %q", disp)
	}
	if cache := res.Cabecera.Get("Cache-Control"); cache != "no-store" {
		t.Errorf("una exportación con datos personales no debería cachearse, llegó %q", cache)
	}
}

// Lo que no es un dato personal no sale: el hash del token de sesión es la
// llave de la cuenta, y el snapshot de un quiz es el examen del profesor.
func TestLaExportacionNoFiltraSecretosNiExamenes(t *testing.T) {
	env := nuevoEntorno(t)
	recursoID, cursoID := env.cursoConQuiz("prof@example.com", "curso-export")
	estudiante := env.inscribir("ana@example.com", cursoID)

	intento := estudiante.hacer(http.MethodPost, "/resources/"+recursoID+"/quiz/attempts", nil)
	attemptID, _ := intento.campo(t, "attempt_id").(string)
	responderTodoBien(t, estudiante, attemptID, preguntasDelIntento(t, intento))
	estudiante.hacer(http.MethodPost, "/quiz/attempts/"+attemptID+"/submit", nil)

	res := estudiante.hacer(http.MethodGet, "/me/export", nil)
	if res.Estado != http.StatusOK {
		t.Fatalf("exportar: %d %s", res.Estado, res.Crudo)
	}

	// Sí está su resultado...
	if !contiene(res.Crudo, "intentos_de_evaluacion") {
		t.Errorf("debería incluir sus intentos: %s", res.Crudo)
	}
	// ...pero no el enunciado ni la clave.
	for _, prohibido := range []string{"token_hash", "password_hash", "is_correct", "CorrectAnswers", "snapshot"} {
		if contiene(res.Crudo, prohibido) {
			t.Errorf("la exportación filtra %q: %s", prohibido, res.Crudo)
		}
	}
}

func TestLaExportacionExigeSesion(t *testing.T) {
	env := nuevoEntorno(t)
	if res := env.cliente().hacer(http.MethodGet, "/me/export", nil); res.Estado != http.StatusUnauthorized {
		t.Errorf("sin sesión debería ser 401, llegó %d", res.Estado)
	}
}
