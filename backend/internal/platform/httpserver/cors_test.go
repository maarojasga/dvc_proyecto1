package httpserver_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/DES-SOLUCIONES-CLOUD/proyecto-1/backend/internal/platform/httpserver"
)

// permitidas parsea la cabecera del preflight en un conjunto comparable.
func permitidas(valor string) map[string]bool {
	out := map[string]bool{}
	for _, c := range strings.Split(valor, ",") {
		out[strings.ToLower(strings.TrimSpace(c))] = true
	}
	return out
}

func TestElPreflightPermiteLasCabecerasQueLaAPIExige(t *testing.T) {
	env := nuevoEntorno(t)

	res := env.cliente().hacer(http.MethodOptions, "/auth/login", nil,
		[2]string{"Origin", "http://localhost:3000"},
		[2]string{"Access-Control-Request-Method", "POST"},
		[2]string{"Access-Control-Request-Headers", "content-type,x-csrf-token"},
	)
	if res.Estado != http.StatusNoContent {
		t.Fatalf("el preflight debería responder 204, llegó %d", res.Estado)
	}

	permite := permitidas(res.Cabecera.Get("Access-Control-Allow-Headers"))
	// El navegador bloquea la petición entera si una sola cabecera
	// falta aquí. X-CSRF-Token la exige el propio middleware CSRF, así que
	// omitirla dejaba inservible todo inicio de sesión desde el navegador.
	for _, obligatoria := range []string{
		"content-type", httpserver.CSRFHeaderName, httpserver.CabeceraIdempotencia,
	} {
		if !permite[strings.ToLower(obligatoria)] {
			t.Errorf("el preflight no permite %q; permite %q",
				obligatoria, res.Cabecera.Get("Access-Control-Allow-Headers"))
		}
	}

	if res.Cabecera.Get("Access-Control-Allow-Credentials") != "true" {
		t.Error("la sesión viaja en cookie: el preflight debe permitir credenciales")
	}
	if !strings.Contains(res.Cabecera.Get("Vary"), "Origin") {
		t.Error("la respuesta depende del origen y debe declararlo en Vary")
	}
}

func TestElClientePuedeLeerLaMarcaDeRepeticion(t *testing.T) {
	env := nuevoEntorno(t)
	res := env.cliente().hacer(http.MethodGet, "/health", nil,
		[2]string{"Origin", "http://localhost:3000"})

	expone := permitidas(res.Cabecera.Get("Access-Control-Expose-Headers"))
	// Sin declararla, el navegador oculta la cabecera aunque viaje, y el
	// cliente no puede distinguir una respuesta repetida de una nueva.
	if !expone[strings.ToLower(httpserver.CabeceraIdempotenciaRepetida)] {
		t.Errorf("el cliente debería poder leer %q; se exponen %q",
			httpserver.CabeceraIdempotenciaRepetida, res.Cabecera.Get("Access-Control-Expose-Headers"))
	}
}

func TestTodaCabeceraQueLaAPIExigeEstaPermitida(t *testing.T) {
	// Guarda contra la deriva que causó el fallo: si mañana se añade una
	// cabecera obligatoria y se olvida la lista de CORS, esto lo detecta.
	permite := map[string]bool{}
	for _, c := range httpserver.CabecerasPermitidas {
		permite[strings.ToLower(c)] = true
	}
	for _, exigida := range []string{httpserver.CSRFHeaderName, httpserver.CabeceraIdempotencia} {
		if !permite[strings.ToLower(exigida)] {
			t.Errorf("%q la exige la API pero no está en CabecerasPermitidas", exigida)
		}
	}
}

func TestLaIPRegistradaNoLlevaPuerto(t *testing.T) {
	env := nuevoEntorno(t)
	env.registrarYVerificar("ana@example.com")
	env.entrar("ana@example.com", clavePrueba)

	// El cliente de pruebas llega desde 192.0.2.10:1234. El puerto es efímero:
	// guardarlo ensucia la bitácora e impide agrupar por dirección.
	ip := env.consultarTexto(
		`SELECT coalesce(ip_address, '') FROM audit_logs WHERE action='auth.login.succeeded' LIMIT 1`)
	if ip != "192.0.2.10" {
		t.Errorf("ip_address = %q; se esperaba la dirección sin puerto", ip)
	}
}
