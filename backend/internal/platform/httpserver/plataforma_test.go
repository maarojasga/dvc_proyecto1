package httpserver_test

import (
	"context"
	"net/http"
	"strings"
	"testing"
)

func TestIdempotencyKeyNoRepiteElAlta(t *testing.T) {
	env := nuevoEntorno(t)
	c := env.cliente()

	alta := map[string]string{"correo": "ana@example.com", "nombre": "Ana", "clave": clavePrueba}
	clave := [2]string{"Idempotency-Key", "alta-de-ana-0001"}

	primera := c.hacer(http.MethodPost, "/auth/registro", alta, clave)
	if primera.Estado != http.StatusAccepted {
		t.Fatalf("estado %d, cuerpo %s", primera.Estado, primera.Crudo)
	}
	if primera.Cabecera.Get("Idempotency-Replayed") != "" {
		t.Error("la primera peticion no deberia venir marcada como repeticion")
	}

	// Reintento del cliente tras un timeout de red: misma clave, mismo cuerpo.
	segunda := c.hacer(http.MethodPost, "/auth/registro", alta, clave)
	if segunda.Estado != primera.Estado || segunda.Crudo != primera.Crudo {
		t.Errorf("la repeticion deberia devolver la respuesta guardada:\n%d %s\n%d %s",
			primera.Estado, primera.Crudo, segunda.Estado, segunda.Crudo)
	}
	if segunda.Cabecera.Get("Idempotency-Replayed") != "true" {
		t.Error("la repeticion deberia venir marcada como tal")
	}

	// Y sobre todo: no se ejecuto dos veces. Un solo correo de verificacion.
	if n := env.correos.totalEntregados(); n != 1 {
		t.Errorf("se esperaba 1 correo enviado, se enviaron %d", n)
	}
	if n := env.contar(`SELECT count(*) FROM usuarios`); n != 1 {
		t.Errorf("se esperaba 1 cuenta, hay %d", n)
	}
}

func TestClavesDeIdempotenciaDistintasSonOperacionesDistintas(t *testing.T) {
	env := nuevoEntorno(t)
	c := env.cliente()

	c.hacer(http.MethodPost, "/auth/registro",
		map[string]string{"correo": "ana@example.com", "nombre": "Ana", "clave": clavePrueba},
		[2]string{"Idempotency-Key", "alta-uno"})
	c.hacer(http.MethodPost, "/auth/registro",
		map[string]string{"correo": "beto@example.com", "nombre": "Beto", "clave": clavePrueba},
		[2]string{"Idempotency-Key", "alta-dos"})

	if n := env.contar(`SELECT count(*) FROM usuarios`); n != 2 {
		t.Errorf("se esperaban 2 cuentas, hay %d", n)
	}
}

func TestLimiteDeTasaFrenaElRociadoDeClaves(t *testing.T) {
	env := nuevoEntorno(t)
	env.registrarYActivar("ana@example.com", clavePrueba)
	c := env.cliente()

	// El cupo de login es de 10 por minuto y por IP. Al pasarlo, la API
	// responde 429 en lugar de seguir aceptando intentos.
	var ultimo int
	for i := 0; i < 12; i++ {
		res := c.hacer(http.MethodPost, "/auth/login", map[string]string{
			"correo": "ana@example.com", "clave": "clave-equivocada-1",
		})
		ultimo = res.Estado
		if res.Estado == http.StatusTooManyRequests {
			if res.Cabecera.Get("Retry-After") == "" {
				t.Error("la respuesta 429 deberia indicar Retry-After")
			}
			return
		}
	}
	t.Errorf("nunca se aplico el limite de tasa; ultimo estado %d", ultimo)
}

func TestLaAuditoriaEsInmutable(t *testing.T) {
	env := nuevoEntorno(t)
	env.registrarYActivar("ana@example.com", clavePrueba)

	if n := env.contar(`SELECT count(*) FROM auditoria`); n == 0 {
		t.Fatal("el alta deberia haber dejado rastro en la auditoria")
	}

	ctx := context.Background()
	// La bitacora solo admite INSERT: el disparador rechaza UPDATE y DELETE
	// incluso para el dueno de la tabla.
	if _, err := env.pool.Exec(ctx, `UPDATE auditoria SET accion = 'falsificada'`); err == nil {
		t.Error("modificar la auditoria deberia fallar")
	} else if !strings.Contains(err.Error(), "inmutable") {
		t.Errorf("el error deberia explicar la inmutabilidad, llego: %v", err)
	}
	if _, err := env.pool.Exec(ctx, `DELETE FROM auditoria`); err == nil {
		t.Error("borrar la auditoria deberia fallar")
	}
}

func TestCuerpoInvalidoDevuelveProblemDetails(t *testing.T) {
	env := nuevoEntorno(t)
	c := env.cliente()

	// Campo desconocido: el decodificador lo rechaza para que un typo del
	// cliente no pase inadvertido.
	res := c.hacer(http.MethodPost, "/auth/login", map[string]string{
		"correo": "ana@example.com", "contrasena": clavePrueba,
	})
	if res.Estado != http.StatusBadRequest {
		t.Fatalf("estado %d, cuerpo %s", res.Estado, res.Crudo)
	}
	if tipo := res.Cabecera.Get("Content-Type"); tipo != "application/problem+json; charset=utf-8" {
		t.Errorf("Content-Type = %q", tipo)
	}
	if res.campo(t, "status") != float64(http.StatusBadRequest) {
		t.Errorf("el problem details deberia repetir el estado: %s", res.Crudo)
	}
	if res.campo(t, "type") == nil || res.campo(t, "title") == nil {
		t.Errorf("faltan campos obligatorios de RFC 9457: %s", res.Crudo)
	}
}

func TestCabecerasDeSeguridadYCorrelacion(t *testing.T) {
	env := nuevoEntorno(t)
	res := env.cliente().hacer(http.MethodGet, "/health", nil)
	if res.Estado != http.StatusOK {
		t.Fatalf("estado %d", res.Estado)
	}
	esperadas := map[string]string{
		"X-Content-Type-Options":  "nosniff",
		"X-Frame-Options":         "DENY",
		"Referrer-Policy":         "no-referrer",
		"Content-Security-Policy": "default-src 'none'; frame-ancestors 'none'",
	}
	for cabecera, valor := range esperadas {
		if got := res.Cabecera.Get(cabecera); got != valor {
			t.Errorf("%s = %q; se esperaba %q", cabecera, got, valor)
		}
	}
	if res.Cabecera.Get("X-Request-Id") == "" {
		t.Error("cada respuesta deberia llevar X-Request-Id para correlacionar logs")
	}
}

func TestRutaDesconocidaDevuelveProblemDetails(t *testing.T) {
	env := nuevoEntorno(t)
	res := env.cliente().hacer(http.MethodGet, "/no-existe", nil)
	if res.Estado != http.StatusNotFound {
		t.Fatalf("estado %d, cuerpo %s", res.Estado, res.Crudo)
	}
	if res.campo(t, "title") == nil {
		t.Errorf("se esperaba un problem details: %s", res.Crudo)
	}
}
