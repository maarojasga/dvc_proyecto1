package httpserver_test

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// Cubren el último punto del alcance opcional 5.2: subtítulos,
// transcripciones, foros asíncronos y Open Badges 3.0.

const pistaVTT = "WEBVTT\n\n" +
	"00:00:00.000 --> 00:00:03.000\nBienvenidos al curso\n\n" +
	"00:00:03.000 --> 00:00:06.000\nHoy hablaremos de\nalmacenamiento\n"

// --- Subtítulos y transcripciones ---

func TestSubirUnaPistaDerivaLaTranscripcion(t *testing.T) {
	env := nuevoEntorno(t)
	c, cursoID, recursoID := env.cursoPublicadoConLectura("prof@example.com", "curso-vtt")
	versionID := env.consultarTexto(
		`SELECT current_published_version_id::text FROM courses WHERE id = $1`, cursoID)
	base := "/courses/versions/" + versionID + "/resources/" + recursoID + "/captions/es"

	res := c.hacer(http.MethodPut, base, map[string]any{
		"label": "Español", "kind": "captions", "vtt": pistaVTT,
	})
	if res.Estado != http.StatusOK {
		t.Fatalf("guardar subtítulos: %d %s", res.Estado, res.Crudo)
	}
	if cues, _ := res.campo(t, "cues").(float64); cues != 2 {
		t.Errorf("se esperaban 2 fragmentos: %s", res.Crudo)
	}

	// La transcripción no se pide aparte: sale de la propia pista.
	estudiante := env.inscribir("ana@example.com", cursoID)
	texto := estudiante.hacer(http.MethodGet, "/resources/"+recursoID+"/transcript/es", nil)
	if texto.Estado != http.StatusOK {
		t.Fatalf("transcripción: %d %s", texto.Estado, texto.Crudo)
	}
	transcripcion, _ := texto.campo(t, "transcript").(string)
	if !contiene(transcripcion, "Bienvenidos al curso") || !contiene(transcripcion, "almacenamiento") {
		t.Errorf("transcripción incompleta: %q", transcripcion)
	}
}

func TestUnArchivoQueNoEsWebVTTSeRechaza(t *testing.T) {
	env := nuevoEntorno(t)
	c, cursoID, recursoID := env.cursoPublicadoConLectura("prof@example.com", "curso-vtt")
	versionID := env.consultarTexto(
		`SELECT current_published_version_id::text FROM courses WHERE id = $1`, cursoID)

	// Un SRT, que es casi idéntico pero el navegador no acepta.
	res := c.hacer(http.MethodPut,
		"/courses/versions/"+versionID+"/resources/"+recursoID+"/captions/es",
		map[string]any{"vtt": "1\n00:00:00,000 --> 00:00:03,000\nHola\n"})
	if res.Estado != http.StatusUnprocessableEntity {
		t.Fatalf("se esperaba 422, llegó %d: %s", res.Estado, res.Crudo)
	}
	if codigo, _ := res.campo(t, "error", "code").(string); codigo != "not_webvtt" {
		t.Errorf("código inesperado: %s", res.Crudo)
	}
}

// Los subtítulos son parte del material: quien no puede ver el recurso tampoco
// puede leerlos.
func TestLosSubtitulosExigenElMismoAccesoQueElRecurso(t *testing.T) {
	env := nuevoEntorno(t)
	c, cursoID, recursoID := env.cursoPublicadoConLectura("prof@example.com", "curso-vtt")
	versionID := env.consultarTexto(
		`SELECT current_published_version_id::text FROM courses WHERE id = $1`, cursoID)
	c.hacer(http.MethodPut,
		"/courses/versions/"+versionID+"/resources/"+recursoID+"/captions/es",
		map[string]any{"vtt": pistaVTT})

	env.registrarYVerificar("curiosa@example.com")
	sinInscribir := env.entrar("curiosa@example.com", clavePrueba)

	if res := sinInscribir.hacer(http.MethodGet, "/resources/"+recursoID+"/captions", nil); res.Estado == http.StatusOK {
		t.Errorf("sin inscripción no debería ver las pistas, llegó %d", res.Estado)
	}
	if res := sinInscribir.hacer(http.MethodGet, "/resources/"+recursoID+"/transcript/es", nil); res.Estado == http.StatusOK {
		t.Errorf("sin inscripción no debería leer la transcripción, llegó %d", res.Estado)
	}
}

// Volver a subir el mismo idioma corrige la pista; no añade una segunda que el
// reproductor no sabría cuál elegir.
func TestSubirDeNuevoElMismoIdiomaReemplazaLaPista(t *testing.T) {
	env := nuevoEntorno(t)
	c, cursoID, recursoID := env.cursoPublicadoConLectura("prof@example.com", "curso-vtt")
	versionID := env.consultarTexto(
		`SELECT current_published_version_id::text FROM courses WHERE id = $1`, cursoID)
	base := "/courses/versions/" + versionID + "/resources/" + recursoID + "/captions/es"

	c.hacer(http.MethodPut, base, map[string]any{"vtt": pistaVTT})
	c.hacer(http.MethodPut, base, map[string]any{
		"vtt": "WEBVTT\n\n00:00:00.000 --> 00:00:02.000\nCorregido\n",
	})

	if n := env.contar(`SELECT count(*) FROM resource_captions WHERE resource_id = $1`, recursoID); n != 1 {
		t.Errorf("se acumularon %d pistas del mismo idioma", n)
	}
	estudiante := env.inscribir("ana@example.com", cursoID)
	texto := estudiante.hacer(http.MethodGet, "/resources/"+recursoID+"/transcript/es", nil)
	if transcripcion, _ := texto.campo(t, "transcript").(string); transcripcion != "Corregido" {
		t.Errorf("la pista no se reemplazó: %q", transcripcion)
	}
}

// --- Foros asíncronos ---

func TestUnEstudianteInscritoAbreYRespondeUnHilo(t *testing.T) {
	env := nuevoEntorno(t)
	_, cursoID, _ := env.cursoPublicadoConLectura("prof@example.com", "curso-foro")
	ana := env.inscribir("ana@example.com", cursoID)
	luis := env.inscribir("luis@example.com", cursoID)

	res := ana.hacer(http.MethodPost, "/forum/"+cursoID, map[string]any{
		"title": "¿Cómo se calcula el progreso?", "body_md": "No me queda claro.",
	})
	if res.Estado != http.StatusCreated {
		t.Fatalf("crear hilo: %d %s", res.Estado, res.Crudo)
	}
	hiloID, _ := res.campo(t, "id").(string)

	if respuesta := luis.hacer(http.MethodPost, "/forum/threads/"+hiloID+"/replies",
		map[string]any{"body_md": "Lo calcula el servidor con los heartbeats."}); respuesta.Estado != http.StatusCreated {
		t.Fatalf("responder: %d %s", respuesta.Estado, respuesta.Crudo)
	}

	hilo := ana.hacer(http.MethodGet, "/forum/threads/"+hiloID, nil)
	if hilo.Estado != http.StatusOK {
		t.Fatalf("ver hilo: %d %s", hilo.Estado, hilo.Crudo)
	}
	respuestas, _ := hilo.campo(t, "replies").([]any)
	if len(respuestas) != 1 {
		t.Fatalf("se esperaba una respuesta: %s", hilo.Crudo)
	}
	if !contiene(hilo.Crudo, "heartbeats") {
		t.Errorf("la respuesta no llegó: %s", hilo.Crudo)
	}
}

// El foro es del curso: no es un tablón público.
func TestQuienNoEstaInscritoNoEntraAlForo(t *testing.T) {
	env := nuevoEntorno(t)
	_, cursoID, _ := env.cursoPublicadoConLectura("prof@example.com", "curso-foro")
	env.registrarYVerificar("curiosa@example.com")
	fuera := env.entrar("curiosa@example.com", clavePrueba)

	if res := fuera.hacer(http.MethodGet, "/forum/"+cursoID, nil); res.Estado == http.StatusOK {
		t.Errorf("sin inscripción no debería ver el foro, llegó %d", res.Estado)
	}
	if res := fuera.hacer(http.MethodPost, "/forum/"+cursoID,
		map[string]any{"title": "Spam", "body_md": "Hola"}); res.Estado == http.StatusCreated {
		t.Errorf("sin inscripción no debería abrir hilos, llegó %d", res.Estado)
	}
}

func TestElProfesorCierraUnHiloYNadieMasResponde(t *testing.T) {
	env := nuevoEntorno(t)
	profesor, cursoID, _ := env.cursoPublicadoConLectura("prof@example.com", "curso-foro")
	ana := env.inscribir("ana@example.com", cursoID)

	res := ana.hacer(http.MethodPost, "/forum/"+cursoID,
		map[string]any{"title": "Hilo", "body_md": "Texto"})
	hiloID, _ := res.campo(t, "id").(string)

	if cerrar := profesor.hacer(http.MethodPost, "/forum/threads/"+hiloID+"/lock",
		map[string]any{"locked": true}); cerrar.Estado != http.StatusOK {
		t.Fatalf("cerrar hilo: %d %s", cerrar.Estado, cerrar.Crudo)
	}

	respuesta := ana.hacer(http.MethodPost, "/forum/threads/"+hiloID+"/replies",
		map[string]any{"body_md": "Una más"})
	if respuesta.Estado != http.StatusConflict {
		t.Errorf("un hilo cerrado no debería aceptar respuestas, llegó %d: %s", respuesta.Estado, respuesta.Crudo)
	}
	// Y un estudiante no puede reabrirlo.
	if reabrir := ana.hacer(http.MethodPost, "/forum/threads/"+hiloID+"/lock",
		map[string]any{"locked": false}); reabrir.Estado == http.StatusOK {
		t.Errorf("un estudiante no debería poder reabrir el hilo, llegó %d", reabrir.Estado)
	}
}

// Borrar una respuesta la oculta pero conserva su lugar: quitar un mensaje
// intermedio dejaría sin sentido a los que le respondían.
func TestBorrarUnaRespuestaLaOcultaSinPartirLaConversacion(t *testing.T) {
	env := nuevoEntorno(t)
	_, cursoID, _ := env.cursoPublicadoConLectura("prof@example.com", "curso-foro")
	ana := env.inscribir("ana@example.com", cursoID)

	res := ana.hacer(http.MethodPost, "/forum/"+cursoID,
		map[string]any{"title": "Hilo", "body_md": "Texto"})
	hiloID, _ := res.campo(t, "id").(string)

	creada := ana.hacer(http.MethodPost, "/forum/threads/"+hiloID+"/replies",
		map[string]any{"body_md": "Un comentario que luego borro"})
	postID, _ := creada.campo(t, "id").(string)

	if borrar := ana.hacer(http.MethodDelete, "/forum/posts/"+postID, nil); borrar.Estado != http.StatusNoContent {
		t.Fatalf("borrar respuesta: %d %s", borrar.Estado, borrar.Crudo)
	}

	hilo := ana.hacer(http.MethodGet, "/forum/threads/"+hiloID, nil)
	respuestas, _ := hilo.campo(t, "replies").([]any)
	if len(respuestas) != 1 {
		t.Fatalf("la respuesta borrada debería seguir ocupando su lugar: %s", hilo.Crudo)
	}
	borrada, _ := respuestas[0].(map[string]any)
	if eliminado, _ := borrada["deleted"].(bool); !eliminado {
		t.Errorf("la respuesta no quedó marcada como eliminada: %s", hilo.Crudo)
	}
	if contiene(hilo.Crudo, "que luego borro") {
		t.Errorf("el cuerpo de una respuesta borrada no debería entregarse: %s", hilo.Crudo)
	}
}

func TestNadieBorraLaRespuestaDeOtroEstudiante(t *testing.T) {
	env := nuevoEntorno(t)
	_, cursoID, _ := env.cursoPublicadoConLectura("prof@example.com", "curso-foro")
	ana := env.inscribir("ana@example.com", cursoID)
	luis := env.inscribir("luis@example.com", cursoID)

	res := ana.hacer(http.MethodPost, "/forum/"+cursoID,
		map[string]any{"title": "Hilo", "body_md": "Texto"})
	hiloID, _ := res.campo(t, "id").(string)
	creada := ana.hacer(http.MethodPost, "/forum/threads/"+hiloID+"/replies",
		map[string]any{"body_md": "Mi comentario"})
	postID, _ := creada.campo(t, "id").(string)

	if borrar := luis.hacer(http.MethodDelete, "/forum/posts/"+postID, nil); borrar.Estado != http.StatusForbidden {
		t.Errorf("otro estudiante no debería borrarla, llegó %d", borrar.Estado)
	}
}

// --- Open Badges 3.0 ---

func TestLaInsigniaSeEmiteComoCredencialOpenBadges(t *testing.T) {
	env := nuevoEntorno(t)
	codigo := env.insigniaEmitida()

	res := env.cliente().hacer(http.MethodGet, "/badges/"+codigo+"/openbadge", nil)
	if res.Estado != http.StatusOK {
		t.Fatalf("credencial: %d %s", res.Estado, res.Crudo)
	}
	for _, esperado := range []string{"VerifiableCredential", "AchievementCredential", "ob/v3p0"} {
		if !contiene(res.Crudo, esperado) {
			t.Errorf("la credencial no declara %q: %s", esperado, res.Crudo)
		}
	}
	// Se comparte, así que no puede llevar nada de la persona.
	if contiene(res.Crudo, "@example.com") || contiene(res.Crudo, "student_id") {
		t.Errorf("la credencial expone datos personales: %s", res.Crudo)
	}
}

func TestLaCredencialFirmadaSeVerificaConElJWKSPublicado(t *testing.T) {
	env := nuevoEntorno(t)
	codigo := env.insigniaEmitida()

	firmada := env.cliente().hacer(http.MethodGet, "/badges/"+codigo+"/openbadge.jwt", nil)
	if firmada.Estado != http.StatusOK {
		t.Fatalf("credencial firmada: %d %s", firmada.Estado, firmada.Crudo)
	}
	jwt := strings.TrimSpace(firmada.Crudo)
	partes := strings.Split(jwt, ".")
	if len(partes) != 3 {
		t.Fatalf("no parece un JWS: %q", jwt)
	}

	// El JWKS es público: el sentido de una credencial verificable es que
	// cualquiera la compruebe sin tener cuenta aquí.
	jwks := env.cliente().hacer(http.MethodGet, "/.well-known/jwks.json", nil)
	if jwks.Estado != http.StatusOK {
		t.Fatalf("jwks: %d %s", jwks.Estado, jwks.Crudo)
	}
	claves, _ := jwks.campo(t, "keys").([]any)
	if len(claves) != 1 {
		t.Fatalf("se esperaba una clave publicada: %s", jwks.Crudo)
	}
	clave, _ := claves[0].(map[string]any)
	if clave["crv"] != "Ed25519" || clave["alg"] != "EdDSA" {
		t.Errorf("el JWKS no describe la clave de firma: %s", jwks.Crudo)
	}

	// La carga contiene la credencial.
	crudo, err := base64.RawURLEncoding.DecodeString(partes[1])
	if err != nil {
		t.Fatalf("carga ilegible: %v", err)
	}
	var carga map[string]any
	if err := json.Unmarshal(crudo, &carga); err != nil {
		t.Fatal(err)
	}
	if _, hay := carga["vc"]; !hay {
		t.Errorf("la carga no lleva la credencial: %s", crudo)
	}
}

// Una credencial firmada no se puede desdecir, así que una insignia revocada
// no se emite como tal: para eso está la URL de verificación.
func TestUnaInsigniaRevocadaNoSeEmiteComoCredencial(t *testing.T) {
	env := nuevoEntorno(t)
	codigo := env.insigniaEmitida()
	admin := env.administrador("admin@example.com")

	if res := admin.hacer(http.MethodPost, "/badges/"+codigo+"/revoke",
		map[string]any{"reason": "error administrativo"}); res.Estado != http.StatusOK {
		t.Fatalf("revocar: %d %s", res.Estado, res.Crudo)
	}

	res := env.cliente().hacer(http.MethodGet, "/badges/"+codigo+"/openbadge", nil)
	if res.Estado != http.StatusConflict {
		t.Errorf("una insignia revocada no debería emitirse como credencial, llegó %d: %s", res.Estado, res.Crudo)
	}
	// Pero la verificación pública sigue respondiendo, diciendo que no vale.
	verificacion := env.cliente().hacer(http.MethodGet, "/badges/"+codigo, nil)
	if verificacion.Estado != http.StatusOK {
		t.Errorf("la verificación pública debería seguir resolviendo: %d", verificacion.Estado)
	}
	if valida, _ := verificacion.campo(t, "valid").(bool); valida {
		t.Error("la verificación debería decir que ya no es válida")
	}
}

// insigniaEmitida deja a una estudiante con una insignia y devuelve su código.
func (e *entorno) insigniaEmitida() string {
	e.t.Helper()
	recursoID, cursoID := e.cursoConQuiz("prof@example.com", "curso-insignia")
	estudiante := e.inscribir("ana@example.com", cursoID)

	intento := estudiante.hacer(http.MethodPost, "/resources/"+recursoID+"/quiz/attempts", nil)
	attemptID, _ := intento.campo(e.t, "attempt_id").(string)
	responderTodoBien(e.t, estudiante, attemptID, preguntasDelIntento(e.t, intento))
	estudiante.hacer(http.MethodPost, "/resources/"+recursoID+"/progress",
		map[string]any{"type": "close", "complete": true})
	estudiante.hacer(http.MethodPost, "/quiz/attempts/"+attemptID+"/submit", nil)

	resumen := estudiante.hacer(http.MethodGet, "/enrollments/"+cursoID+"/progress", nil)
	codigo, _ := resumen.campo(e.t, "badge_code").(string)
	if codigo == "" {
		e.t.Fatalf("no se emitió la insignia: %s", resumen.Crudo)
	}
	return codigo
}
