package httpserver_test

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/DES-SOLUCIONES-CLOUD/proyecto-1/backend/internal/platform/httpserver"
)

// idSesionActual lee de la lista cuál es la sesión de este cliente.
func idSesionActual(t *testing.T, c *cliente) string {
	t.Helper()
	res := c.hacer(http.MethodGet, "/auth/sessions", nil)
	if res.Estado != http.StatusOK {
		t.Fatalf("sessions: %d %s", res.Estado, res.Crudo)
	}
	items, _ := res.campo(t, "items").([]any)
	for _, it := range items {
		s, _ := it.(map[string]any)
		if s["current"] == true {
			return s["id"].(string)
		}
	}
	t.Fatalf("ninguna sesión venía marcada como actual: %s", res.Crudo)
	return ""
}

func TestListarSesionesMarcaLaActual(t *testing.T) {
	env := nuevoEntorno(t)
	env.registrarYVerificar("ana@example.com")

	movil := env.entrar("ana@example.com", clavePrueba)
	env.entrar("ana@example.com", clavePrueba) // segundo dispositivo

	res := movil.hacer(http.MethodGet, "/auth/sessions", nil)
	items, _ := res.campo(t, "items").([]any)
	if len(items) != 2 {
		t.Fatalf("se esperaban 2 sesiones activas, hay %d: %s", len(items), res.Crudo)
	}
	actuales := 0
	for _, it := range items {
		if s, _ := it.(map[string]any); s["current"] == true {
			actuales++
		}
	}
	if actuales != 1 {
		t.Errorf("exactamente una sesión debería marcarse como actual, hay %d", actuales)
	}
}

func TestRevocarUnaSesionLaInvalidaDeInmediato(t *testing.T) {
	env := nuevoEntorno(t)
	env.registrarYVerificar("ana@example.com")

	movil := env.entrar("ana@example.com", clavePrueba)
	portatil := env.entrar("ana@example.com", clavePrueba)
	idMovil := idSesionActual(t, movil)

	if res := portatil.hacer(http.MethodDelete, "/auth/sessions/"+idMovil, nil); res.Estado != http.StatusNoContent {
		t.Fatalf("revocación: %d %s", res.Estado, res.Crudo)
	}

	// La siguiente petición del móvil ya no vale: la revocación surte efecto
	// al instante, sin esperar a que expire nada.
	if tras := movil.hacer(http.MethodGet, "/auth/me", nil); tras.Estado != http.StatusUnauthorized {
		t.Errorf("la sesión revocada debería dejar de valer, llegó %d: %s", tras.Estado, tras.Crudo)
	}
	if sigue := portatil.hacer(http.MethodGet, "/auth/me", nil); sigue.Estado != http.StatusOK {
		t.Errorf("la sesión no revocada debería seguir valiendo, llegó %d", sigue.Estado)
	}
	if n := env.contar(`SELECT count(*) FROM audit_logs WHERE action='auth.session.revoked'`); n != 1 {
		t.Errorf("la revocación debería auditarse, hay %d entradas", n)
	}
}

func TestNoSePuedenRevocarSesionesAjenas(t *testing.T) {
	env := nuevoEntorno(t)
	env.registrarYVerificar("ana@example.com")
	env.registrarYVerificar("beto@example.com")

	ana := env.entrar("ana@example.com", clavePrueba)
	beto := env.entrar("beto@example.com", clavePrueba)
	idAna := idSesionActual(t, ana)

	// Beto intenta cerrar la sesión de Ana. Debe responder como si no
	// existiera: ni la revoca ni confirma que el identificador sea real.
	if intento := beto.hacer(http.MethodDelete, "/auth/sessions/"+idAna, nil); intento.Estado != http.StatusNotFound {
		t.Errorf("%d %s", intento.Estado, intento.Crudo)
	}
	if sigue := ana.hacer(http.MethodGet, "/auth/me", nil); sigue.Estado != http.StatusOK {
		t.Error("la sesión de Ana no debería verse afectada")
	}
}

func TestRevocarLasDemasSesionesDejaSoloLaActual(t *testing.T) {
	env := nuevoEntorno(t)
	env.registrarYVerificar("ana@example.com")

	uno := env.entrar("ana@example.com", clavePrueba)
	dos := env.entrar("ana@example.com", clavePrueba)
	tres := env.entrar("ana@example.com", clavePrueba)

	res := tres.hacer(http.MethodDelete, "/auth/sessions", nil)
	if res.Estado != http.StatusOK {
		t.Fatalf("%d %s", res.Estado, res.Crudo)
	}
	if n := res.campo(t, "revoked"); n != float64(2) {
		t.Errorf("se esperaban 2 revocadas, llegó %v", n)
	}
	for nombre, c := range map[string]*cliente{"uno": uno, "dos": dos} {
		if r := c.hacer(http.MethodGet, "/auth/me", nil); r.Estado != http.StatusUnauthorized {
			t.Errorf("la sesión %s debería haber quedado revocada, llegó %d", nombre, r.Estado)
		}
	}
	if r := tres.hacer(http.MethodGet, "/auth/me", nil); r.Estado != http.StatusOK {
		t.Error("la sesión actual debería sobrevivir")
	}
}

func TestRevocarLaSesionActualEquivaleACerrarla(t *testing.T) {
	env := nuevoEntorno(t)
	env.registrarYVerificar("ana@example.com")
	c := env.entrar("ana@example.com", clavePrueba)

	if res := c.hacer(http.MethodDelete, "/auth/sessions/"+idSesionActual(t, c), nil); res.Estado != http.StatusNoContent {
		t.Fatalf("%d %s", res.Estado, res.Crudo)
	}
	if _, sigue := c.cookies[httpserver.SessionCookieName]; sigue {
		t.Error("debería borrarse la cookie de sesión")
	}
}

func TestCerrarSesionRevocaSoloLaActual(t *testing.T) {
	env := nuevoEntorno(t)
	env.registrarYVerificar("ana@example.com")

	movil := env.entrar("ana@example.com", clavePrueba)
	portatil := env.entrar("ana@example.com", clavePrueba)

	if res := movil.hacer(http.MethodPost, "/auth/logout", nil); res.Estado != http.StatusOK {
		t.Fatalf("logout: %d %s", res.Estado, res.Crudo)
	}
	if _, sigue := movil.cookies[httpserver.SessionCookieName]; sigue {
		t.Error("el logout debería borrar la cookie de sesión")
	}
	if res := portatil.hacer(http.MethodGet, "/auth/me", nil); res.Estado != http.StatusOK {
		t.Error("el otro dispositivo debería seguir con sesión")
	}
}

func TestRecuperacionCambiaLaClaveYCierraTodasLasSesiones(t *testing.T) {
	env := nuevoEntorno(t)
	env.registrarYVerificar("ana@example.com")

	movil := env.entrar("ana@example.com", clavePrueba)
	portatil := env.entrar("ana@example.com", clavePrueba)

	c := env.cliente()
	if res := c.hacer(http.MethodPost, "/auth/password/reset-request", map[string]string{"email": "ana@example.com"}); res.Estado != http.StatusAccepted {
		t.Fatalf("solicitud: %d %s", res.Estado, res.Crudo)
	}
	token := env.correos.tokenPara(t, "ana@example.com")
	if res := c.hacer(http.MethodPost, "/auth/password/reset-confirm", map[string]string{
		"token": token, "new_password": claveNueva,
	}); res.Estado != http.StatusOK {
		t.Fatalf("confirmación: %d %s", res.Estado, res.Crudo)
	}

	// Recuperar la clave expulsa a todo el mundo, incluido un posible intruso.
	for nombre, s := range map[string]*cliente{"móvil": movil, "portátil": portatil} {
		if r := s.hacer(http.MethodGet, "/auth/me", nil); r.Estado != http.StatusUnauthorized {
			t.Errorf("la sesión %s debería haberse cerrado, llegó %d", nombre, r.Estado)
		}
	}
	if r := env.cliente().hacer(http.MethodPost, "/auth/login", map[string]string{
		"email": "ana@example.com", "password": clavePrueba,
	}); r.Estado != http.StatusUnauthorized {
		t.Errorf("la clave antigua no debería servir, llegó %d", r.Estado)
	}
	env.entrar("ana@example.com", claveNueva)
}

func TestTokenDeRecuperacionEsDeUnSoloUso(t *testing.T) {
	env := nuevoEntorno(t)
	env.registrarYVerificar("ana@example.com")
	c := env.cliente()

	c.hacer(http.MethodPost, "/auth/password/reset-request", map[string]string{"email": "ana@example.com"})
	token := env.correos.tokenPara(t, "ana@example.com")

	if res := c.hacer(http.MethodPost, "/auth/password/reset-confirm", map[string]string{
		"token": token, "new_password": claveNueva,
	}); res.Estado != http.StatusOK {
		t.Fatalf("%d %s", res.Estado, res.Crudo)
	}
	if res := c.hacer(http.MethodPost, "/auth/password/reset-confirm", map[string]string{
		"token": token, "new_password": "Tercera-Clave-Distinta-3",
	}); res.Estado != http.StatusBadRequest {
		t.Errorf("reusar el token debería fallar, llegó %d: %s", res.Estado, res.Crudo)
	}
	env.entrar("ana@example.com", claveNueva)
}

func TestSolicitarRecuperacionNoRevelaSiLaCuentaExiste(t *testing.T) {
	env := nuevoEntorno(t)
	env.registrarYVerificar("ana@example.com")
	c := env.cliente()

	existente := c.hacer(http.MethodPost, "/auth/password/reset-request", map[string]string{"email": "ana@example.com"})
	inexistente := c.hacer(http.MethodPost, "/auth/password/reset-request", map[string]string{"email": "nadie@example.com"})

	if existente.Estado != inexistente.Estado || existente.Crudo != inexistente.Crudo {
		t.Errorf("las respuestas permiten enumerar cuentas:\n%d %s\n%d %s",
			existente.Estado, existente.Crudo, inexistente.Estado, inexistente.Crudo)
	}
	if n := env.contar(`SELECT count(*) FROM password_reset_tokens`); n != 1 {
		t.Errorf("solo debería emitirse un token, hay %d", n)
	}
}

func TestOperacionesQueCambianEstadoExigenTokenCSRF(t *testing.T) {
	env := nuevoEntorno(t)
	env.registrarYVerificar("ana@example.com")
	c := env.entrar("ana@example.com", clavePrueba)

	// Sin la cabecera, que un sitio atacante no puede fijar aunque provoque
	// la petición con la cookie del usuario.
	if sin := c.hacer(http.MethodPost, "/auth/logout", nil, [2]string{httpserver.CSRFHeaderName, ""}); sin.Estado != http.StatusForbidden {
		t.Fatalf("sin cabecera: %d %s", sin.Estado, sin.Crudo)
	}
	// Con un valor que no coincide con la cookie.
	if mal := c.hacer(http.MethodPost, "/auth/logout", nil, [2]string{httpserver.CSRFHeaderName, "valor-inventado"}); mal.Estado != http.StatusForbidden {
		t.Fatalf("cabecera errónea: %d %s", mal.Estado, mal.Crudo)
	}
	// La lectura no cambia estado y no lo exige.
	if get := c.hacer(http.MethodGet, "/auth/me", nil, [2]string{httpserver.CSRFHeaderName, ""}); get.Estado != http.StatusOK {
		t.Errorf("GET no debería exigir CSRF, llegó %d", get.Estado)
	}
	// Y con el token correcto la operación procede.
	if ok := c.hacer(http.MethodPost, "/auth/logout", nil); ok.Estado != http.StatusOK {
		t.Errorf("con CSRF válido debería funcionar, llegó %d: %s", ok.Estado, ok.Crudo)
	}
}

func TestIdempotencyKeyNoRepiteElAlta(t *testing.T) {
	env := nuevoEntorno(t)
	c := env.cliente()
	alta := map[string]string{"email": "ana@example.com", "password": clavePrueba, "full_name": "Ana"}
	clave := [2]string{"Idempotency-Key", "alta-de-ana-0001"}

	primera := c.hacer(http.MethodPost, "/auth/register", alta, clave)
	if primera.Estado != http.StatusAccepted {
		t.Fatalf("%d %s", primera.Estado, primera.Crudo)
	}
	if primera.Cabecera.Get("Idempotency-Replayed") != "" {
		t.Error("la primera petición no debería venir marcada como repetición")
	}

	// Reintento del cliente tras un timeout de red: misma clave, mismo cuerpo.
	segunda := c.hacer(http.MethodPost, "/auth/register", alta, clave)
	if segunda.Estado != primera.Estado || segunda.Crudo != primera.Crudo {
		t.Errorf("la repetición debería devolver la respuesta guardada:\n%d %s\n%d %s",
			primera.Estado, primera.Crudo, segunda.Estado, segunda.Crudo)
	}
	if segunda.Cabecera.Get("Idempotency-Replayed") != "true" {
		t.Error("la repetición debería venir marcada como tal")
	}
	// Lo esencial: no se ejecutó dos veces, así que solo salió un correo.
	if n := env.correos.total(); n != 1 {
		t.Errorf("se esperaba 1 correo enviado, se enviaron %d", n)
	}
}

func TestClavesDeIdempotenciaDistintasSonOperacionesDistintas(t *testing.T) {
	env := nuevoEntorno(t)
	c := env.cliente()
	c.hacer(http.MethodPost, "/auth/register",
		map[string]string{"email": "ana@example.com", "password": clavePrueba, "full_name": "Ana"},
		[2]string{"Idempotency-Key", "alta-uno"})
	c.hacer(http.MethodPost, "/auth/register",
		map[string]string{"email": "beto@example.com", "password": clavePrueba, "full_name": "Beto"},
		[2]string{"Idempotency-Key", "alta-dos"})

	if n := env.contar(`SELECT count(*) FROM users`); n != 2 {
		t.Errorf("se esperaban 2 cuentas, hay %d", n)
	}
}

func TestLaAuditoriaEsInmutable(t *testing.T) {
	env := nuevoEntorno(t)
	env.registrarYVerificar("ana@example.com")
	if n := env.contar(`SELECT count(*) FROM audit_logs`); n == 0 {
		t.Fatal("el alta debería haber dejado rastro en la auditoría")
	}

	ctx := context.Background()
	// La bitácora solo admite INSERT: el disparador rechaza UPDATE y DELETE
	// incluso para el dueño de la tabla.
	if _, err := env.pool.Exec(ctx, `UPDATE audit_logs SET action='falsificada'`); err == nil {
		t.Error("modificar la auditoría debería fallar")
	} else if !strings.Contains(err.Error(), "inmutable") {
		t.Errorf("el error debería explicar la inmutabilidad, llegó: %v", err)
	}
	if _, err := env.pool.Exec(ctx, `DELETE FROM audit_logs`); err == nil {
		t.Error("borrar la auditoría debería fallar")
	}
}

func TestSesionesRequierenAutenticacion(t *testing.T) {
	env := nuevoEntorno(t)
	for _, caso := range []struct{ metodo, ruta string }{
		{http.MethodGet, "/auth/sessions"},
		{http.MethodDelete, "/auth/sessions"},
		{http.MethodGet, "/auth/me"},
	} {
		if res := env.cliente().hacer(caso.metodo, caso.ruta, nil); res.Estado != http.StatusUnauthorized {
			t.Errorf("%s %s: %d %s", caso.metodo, caso.ruta, res.Estado, res.Crudo)
		}
	}
}
