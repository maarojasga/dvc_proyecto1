package httpserver

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// El fallo que motiva esto: X-Forwarded-For se creía sin condiciones, así que
// cualquiera podía rotar el valor y esquivar el límite de tasa por completo,
// además de envenenar la bitácora con direcciones a elección.
func TestSinProxiesDeclaradosNoSeCreeLaCabecera(t *testing.T) {
	sinConfianza := NuevosProxiesDeConfianza("")
	r := httptest.NewRequest(http.MethodPost, "/login", nil)
	r.RemoteAddr = "203.0.113.7:5555"
	r.Header.Set("X-Forwarded-For", "1.2.3.4")

	if got := ipDelCliente(r, sinConfianza); got != "203.0.113.7" {
		t.Fatalf("se atribuyó a %q; debía usar la del socket y no la cabecera", got)
	}
}

func TestDetrasDeUnProxyDeConfianzaSeLeeLaCabecera(t *testing.T) {
	confiables := NuevosProxiesDeConfianza("10.0.0.0/8")
	r := httptest.NewRequest(http.MethodPost, "/login", nil)
	r.RemoteAddr = "10.0.0.5:40000"
	r.Header.Set("X-Forwarded-For", "203.0.113.7")

	if got := ipDelCliente(r, confiables); got != "203.0.113.7" {
		t.Fatalf("se atribuyó a %q, esperaba 203.0.113.7", got)
	}
}

// Se recorre de derecha a izquierda y se toma el primer salto que no
// controlamos. Leer el primer elemento, como se hacía, es leer justo la parte
// que el cliente escribe: aquí el atacante puso "1.2.3.4" delante.
func TestSeTomaElUltimoSaltoQueNoEsDeConfianza(t *testing.T) {
	confiables := NuevosProxiesDeConfianza("10.0.0.0/8, 172.16.0.0/12")
	r := httptest.NewRequest(http.MethodPost, "/login", nil)
	r.RemoteAddr = "10.0.0.5:40000"
	r.Header.Set("X-Forwarded-For", "1.2.3.4, 203.0.113.7, 172.16.0.9")

	if got := ipDelCliente(r, confiables); got != "203.0.113.7" {
		t.Fatalf("se atribuyó a %q, esperaba 203.0.113.7 (el último salto ajeno)", got)
	}
}

// Un valor inventado corta la cadena: a partir de ahí nada es creíble.
func TestUnaCadenaCorruptaCaeALaDireccionDelSocket(t *testing.T) {
	confiables := NuevosProxiesDeConfianza("10.0.0.0/8")
	r := httptest.NewRequest(http.MethodPost, "/login", nil)
	r.RemoteAddr = "10.0.0.5:40000"
	r.Header.Set("X-Forwarded-For", "no-es-una-ip")

	if got := ipDelCliente(r, confiables); got != "10.0.0.5" {
		t.Fatalf("se atribuyó a %q, esperaba la del socket", got)
	}
}

// Varias cabeceras X-Forwarded-For se concatenan, que es lo que hace un
// atacante para partir la cadena en dos.
func TestVariasCabecerasSeTratanComoUnaCadena(t *testing.T) {
	confiables := NuevosProxiesDeConfianza("10.0.0.0/8")
	r := httptest.NewRequest(http.MethodPost, "/login", nil)
	r.RemoteAddr = "10.0.0.5:40000"
	r.Header.Add("X-Forwarded-For", "1.2.3.4")
	r.Header.Add("X-Forwarded-For", "203.0.113.7")

	if got := ipDelCliente(r, confiables); got != "203.0.113.7" {
		t.Fatalf("se atribuyó a %q, esperaba 203.0.113.7", got)
	}
}

func TestUnaPeticionInternaSeAtribuyeAlProxy(t *testing.T) {
	confiables := NuevosProxiesDeConfianza("10.0.0.0/8")
	r := httptest.NewRequest(http.MethodGet, "/health", nil)
	r.RemoteAddr = "10.0.0.5:40000"
	r.Header.Set("X-Forwarded-For", "10.0.0.9")

	if got := ipDelCliente(r, confiables); got != "10.0.0.5" {
		t.Fatalf("se atribuyó a %q; toda la cadena era interna", got)
	}
}

func TestSeAdmitenIPSueltasYSeDescartaLaBasura(t *testing.T) {
	// Una entrada mal escrita reduce la confianza, no la amplía: tratarla como
	// comodín convertiría un dedazo en un agujero.
	p := NuevosProxiesDeConfianza("10.0.0.5, esto-no-es-una-red, 999.999.0.0/16")
	if len(p.redes) != 1 {
		t.Fatalf("se aceptaron %d redes, esperaba solo la válida", len(p.redes))
	}
	r := httptest.NewRequest(http.MethodGet, "/x", nil)
	r.RemoteAddr = "10.0.0.5:1"
	r.Header.Set("X-Forwarded-For", "203.0.113.7")
	if got := ipDelCliente(r, p); got != "203.0.113.7" {
		t.Errorf("una IP suelta debía valer como proxy de confianza; obtuve %q", got)
	}
}

func TestIPv6(t *testing.T) {
	confiables := NuevosProxiesDeConfianza("fd00::/8")
	r := httptest.NewRequest(http.MethodGet, "/x", nil)
	r.RemoteAddr = "[fd00::1]:40000"
	r.Header.Set("X-Forwarded-For", "2001:db8::5")
	if got := ipDelCliente(r, confiables); got != "2001:db8::5" {
		t.Fatalf("se atribuyó a %q, esperaba 2001:db8::5", got)
	}
}

// Las entradas de X-Forwarded-For pueden traer puerto.
func TestSeQuitaElPuertoDeLaCabecera(t *testing.T) {
	confiables := NuevosProxiesDeConfianza("10.0.0.0/8")
	r := httptest.NewRequest(http.MethodGet, "/x", nil)
	r.RemoteAddr = "10.0.0.5:1"
	r.Header.Set("X-Forwarded-For", "203.0.113.7:6543")
	if got := ipDelCliente(r, confiables); got != "203.0.113.7" {
		t.Fatalf("se atribuyó a %q, esperaba sin puerto", got)
	}
}
