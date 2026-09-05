package httpserver_test

import (
	"net/http"
	"testing"

	"github.com/DES-SOLUCIONES-CLOUD/proyecto-1/backend/internal/platform/httpserver"
)

func TestRevocarUnaSesionLaInvalidaDeInmediato(t *testing.T) {
	env := nuevoEntorno(t)
	env.registrarYActivar("ana@example.com", clavePrueba)

	// Dos dispositivos de la misma persona.
	movil := env.entrar("ana@example.com", clavePrueba)
	portatil := env.entrar("ana@example.com", clavePrueba)

	// El movil sigue funcionando: se lee su identificador de sesion.
	sesionMovil := movil.hacer(http.MethodGet, "/auth/sesion", nil)
	idMovil, _ := sesionMovil.campo(t, "sesionId").(string)
	if idMovil == "" {
		t.Fatal("no se pudo leer el identificador de la sesion del movil")
	}

	listado := portatil.hacer(http.MethodGet, "/auth/sesiones", nil)
	items, _ := listado.campo(t, "items").([]any)
	if len(items) != 2 {
		t.Fatalf("se esperaban 2 sesiones activas, hay %d: %s", len(items), listado.Crudo)
	}

	// Desde el portatil se revoca la sesion del movil.
	revocacion := portatil.hacer(http.MethodDelete, "/auth/sesiones/"+idMovil, nil)
	if revocacion.Estado != http.StatusNoContent {
		t.Fatalf("revocacion: estado %d, cuerpo %s", revocacion.Estado, revocacion.Crudo)
	}

	// La siguiente peticion del movil ya no vale. Es la comprobacion clave:
	// la sesion cacheada en Redis debe haberse purgado, no esperar al TTL.
	tras := movil.hacer(http.MethodGet, "/auth/sesion", nil)
	if tras.Estado != http.StatusUnauthorized {
		t.Errorf("la sesion revocada deberia dejar de valer al instante, llego %d: %s", tras.Estado, tras.Crudo)
	}
	// Y el portatil sigue dentro.
	if sigue := portatil.hacer(http.MethodGet, "/auth/sesion", nil); sigue.Estado != http.StatusOK {
		t.Errorf("la sesion no revocada deberia seguir valiendo, llego %d", sigue.Estado)
	}
}

func TestNoSePuedenRevocarSesionesAjenas(t *testing.T) {
	env := nuevoEntorno(t)
	env.registrarYActivar("ana@example.com", clavePrueba)
	env.registrarYActivar("beto@example.com", clavePrueba)

	ana := env.entrar("ana@example.com", clavePrueba)
	beto := env.entrar("beto@example.com", clavePrueba)

	idAna, _ := ana.hacer(http.MethodGet, "/auth/sesion", nil).campo(t, "sesionId").(string)

	// Beto intenta cerrar la sesion de Ana. Debe responder como si no
	// existiera: ni la revoca ni confirma que el identificador sea real.
	intento := beto.hacer(http.MethodDelete, "/auth/sesiones/"+idAna, nil)
	if intento.Estado != http.StatusNotFound {
		t.Errorf("estado %d, cuerpo %s", intento.Estado, intento.Crudo)
	}
	if sigue := ana.hacer(http.MethodGet, "/auth/sesion", nil); sigue.Estado != http.StatusOK {
		t.Error("la sesion de Ana no deberia verse afectada")
	}
}

func TestCerrarSesionRevocaSoloLaActual(t *testing.T) {
	env := nuevoEntorno(t)
	env.registrarYActivar("ana@example.com", clavePrueba)

	movil := env.entrar("ana@example.com", clavePrueba)
	portatil := env.entrar("ana@example.com", clavePrueba)

	if res := movil.hacer(http.MethodPost, "/auth/logout", nil); res.Estado != http.StatusNoContent {
		t.Fatalf("logout: estado %d, cuerpo %s", res.Estado, res.Crudo)
	}
	if _, sigue := movil.cookies[httpserver.CookieSesion]; sigue {
		t.Error("el logout deberia borrar la cookie de sesion")
	}
	if res := portatil.hacer(http.MethodGet, "/auth/sesion", nil); res.Estado != http.StatusOK {
		t.Error("el otro dispositivo deberia seguir con sesion")
	}
}

func TestRevocarLasDemasSesionesDejaSoloLaActual(t *testing.T) {
	env := nuevoEntorno(t)
	env.registrarYActivar("ana@example.com", clavePrueba)

	uno := env.entrar("ana@example.com", clavePrueba)
	dos := env.entrar("ana@example.com", clavePrueba)
	tres := env.entrar("ana@example.com", clavePrueba)

	res := tres.hacer(http.MethodDelete, "/auth/sesiones", nil)
	if res.Estado != http.StatusOK {
		t.Fatalf("estado %d, cuerpo %s", res.Estado, res.Crudo)
	}
	if n := res.campo(t, "revocadas"); n != float64(2) {
		t.Errorf("se esperaban 2 revocadas, llego %v", n)
	}
	for nombre, c := range map[string]*cliente{"uno": uno, "dos": dos} {
		if r := c.hacer(http.MethodGet, "/auth/sesion", nil); r.Estado != http.StatusUnauthorized {
			t.Errorf("la sesion %s deberia haber quedado revocada, llego %d", nombre, r.Estado)
		}
	}
	if r := tres.hacer(http.MethodGet, "/auth/sesion", nil); r.Estado != http.StatusOK {
		t.Error("la sesion actual deberia sobrevivir")
	}
}

func TestSesionesRequierenTokenCSRFParaOperacionesQueCambianEstado(t *testing.T) {
	env := nuevoEntorno(t)
	env.registrarYActivar("ana@example.com", clavePrueba)
	c := env.entrar("ana@example.com", clavePrueba)

	// Sin la cabecera X-CSRF-Token, que un sitio atacante no podria fijar.
	sinCabecera := c.hacer(http.MethodPost, "/auth/logout", nil, [2]string{httpserver.CabeceraCSRF, ""})
	if sinCabecera.Estado != http.StatusForbidden {
		t.Fatalf("estado %d, cuerpo %s", sinCabecera.Estado, sinCabecera.Crudo)
	}
	// Con un valor que no coincide con la cookie.
	noCoincide := c.hacer(http.MethodPost, "/auth/logout", nil, [2]string{httpserver.CabeceraCSRF, "valor-inventado"})
	if noCoincide.Estado != http.StatusForbidden {
		t.Fatalf("estado %d, cuerpo %s", noCoincide.Estado, noCoincide.Crudo)
	}
	// La lectura, que no cambia estado, no exige CSRF.
	if lectura := c.hacer(http.MethodGet, "/auth/sesion", nil, [2]string{httpserver.CabeceraCSRF, ""}); lectura.Estado != http.StatusOK {
		t.Errorf("GET no deberia exigir CSRF, llego %d", lectura.Estado)
	}
	// Y con el token correcto la operacion procede.
	if ok := c.hacer(http.MethodPost, "/auth/logout", nil); ok.Estado != http.StatusNoContent {
		t.Errorf("con CSRF valido deberia funcionar, llego %d: %s", ok.Estado, ok.Crudo)
	}
}
