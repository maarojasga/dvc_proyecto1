package httpserver_test

import (
	"net/http"
	"testing"
)

const claveNueva = "clave-nueva-segura-2"

func TestRecuperacionCambiaLaClaveYCierraTodasLasSesiones(t *testing.T) {
	env := nuevoEntorno(t)
	env.registrarYActivar("ana@example.com", clavePrueba)

	// Dos sesiones abiertas, como si un intruso tuviera una de ellas.
	movil := env.entrar("ana@example.com", clavePrueba)
	portatil := env.entrar("ana@example.com", clavePrueba)

	c := env.cliente()
	res := c.hacer(http.MethodPost, "/auth/recuperar-clave", map[string]string{"correo": "ana@example.com"})
	if res.Estado != http.StatusAccepted {
		t.Fatalf("solicitud: estado %d, cuerpo %s", res.Estado, res.Crudo)
	}
	token := env.correos.tokenRecuperacion(t, "ana@example.com")

	confirmacion := c.hacer(http.MethodPost, "/auth/recuperar-clave/confirmar", map[string]string{
		"token": token, "clave": claveNueva,
	})
	if confirmacion.Estado != http.StatusOK {
		t.Fatalf("confirmacion: estado %d, cuerpo %s", confirmacion.Estado, confirmacion.Crudo)
	}

	// Recuperar la clave expulsa a todo el mundo, incluido el intruso.
	for nombre, sesion := range map[string]*cliente{"movil": movil, "portatil": portatil} {
		if r := sesion.hacer(http.MethodGet, "/auth/sesion", nil); r.Estado != http.StatusUnauthorized {
			t.Errorf("la sesion %s deberia haberse cerrado, llego %d", nombre, r.Estado)
		}
	}

	// La clave antigua ya no sirve y la nueva si.
	if r := env.cliente().hacer(http.MethodPost, "/auth/login", map[string]string{
		"correo": "ana@example.com", "clave": clavePrueba,
	}); r.Estado != http.StatusUnauthorized {
		t.Errorf("la clave antigua no deberia servir, llego %d", r.Estado)
	}
	env.entrar("ana@example.com", claveNueva)
}

func TestTokenDeRecuperacionEsDeUnSoloUso(t *testing.T) {
	env := nuevoEntorno(t)
	env.registrarYActivar("ana@example.com", clavePrueba)

	c := env.cliente()
	c.hacer(http.MethodPost, "/auth/recuperar-clave", map[string]string{"correo": "ana@example.com"})
	token := env.correos.tokenRecuperacion(t, "ana@example.com")

	primera := c.hacer(http.MethodPost, "/auth/recuperar-clave/confirmar", map[string]string{
		"token": token, "clave": claveNueva,
	})
	if primera.Estado != http.StatusOK {
		t.Fatalf("estado %d, cuerpo %s", primera.Estado, primera.Crudo)
	}

	segunda := c.hacer(http.MethodPost, "/auth/recuperar-clave/confirmar", map[string]string{
		"token": token, "clave": "tercera-clave-distinta",
	})
	if segunda.Estado != http.StatusBadRequest {
		t.Errorf("reusar el token deberia fallar, llego %d: %s", segunda.Estado, segunda.Crudo)
	}
	// Y la clave sigue siendo la de la primera confirmacion.
	env.entrar("ana@example.com", claveNueva)
}

func TestSolicitarRecuperacionNoRevelaSiLaCuentaExiste(t *testing.T) {
	env := nuevoEntorno(t)
	env.registrarYActivar("ana@example.com", clavePrueba)
	c := env.cliente()

	existente := c.hacer(http.MethodPost, "/auth/recuperar-clave", map[string]string{"correo": "ana@example.com"})
	inexistente := c.hacer(http.MethodPost, "/auth/recuperar-clave", map[string]string{"correo": "nadie@example.com"})

	if existente.Estado != inexistente.Estado || existente.Crudo != inexistente.Crudo {
		t.Errorf("las respuestas permiten enumerar cuentas:\n%d %s\n%d %s",
			existente.Estado, existente.Crudo, inexistente.Estado, inexistente.Crudo)
	}
	// Pero solo se envia correo a la cuenta que existe.
	if n := env.contar(`SELECT count(*) FROM tokens_unico_uso WHERE proposito = 'recuperacion_clave'`); n != 1 {
		t.Errorf("solo deberia emitirse un token, hay %d", n)
	}
}

func TestEmitirUnNuevoTokenInvalidaElAnterior(t *testing.T) {
	env := nuevoEntorno(t)
	env.registrarYActivar("ana@example.com", clavePrueba)
	c := env.cliente()

	c.hacer(http.MethodPost, "/auth/recuperar-clave", map[string]string{"correo": "ana@example.com"})
	primero := env.correos.tokenRecuperacion(t, "ana@example.com")

	c.hacer(http.MethodPost, "/auth/recuperar-clave", map[string]string{"correo": "ana@example.com"})
	segundo := env.correos.tokenRecuperacion(t, "ana@example.com")

	if primero == segundo {
		t.Fatal("el segundo envio deberia emitir un token distinto")
	}
	res := c.hacer(http.MethodPost, "/auth/recuperar-clave/confirmar", map[string]string{
		"token": primero, "clave": claveNueva,
	})
	if res.Estado != http.StatusBadRequest {
		t.Errorf("el token viejo deberia quedar invalidado, llego %d: %s", res.Estado, res.Crudo)
	}
}

func TestRecuperacionRechazaClaveDebil(t *testing.T) {
	env := nuevoEntorno(t)
	env.registrarYActivar("ana@example.com", clavePrueba)
	c := env.cliente()

	c.hacer(http.MethodPost, "/auth/recuperar-clave", map[string]string{"correo": "ana@example.com"})
	token := env.correos.tokenRecuperacion(t, "ana@example.com")

	res := c.hacer(http.MethodPost, "/auth/recuperar-clave/confirmar", map[string]string{
		"token": token, "clave": "corta",
	})
	if res.Estado != http.StatusUnprocessableEntity {
		t.Fatalf("estado %d, cuerpo %s", res.Estado, res.Crudo)
	}
	// La clave debil no debe consumir el token: el usuario puede reintentar.
	if n := env.contar(`SELECT count(*) FROM tokens_unico_uso WHERE proposito = 'recuperacion_clave' AND usado_en IS NULL`); n != 1 {
		t.Errorf("el token deberia seguir disponible, hay %d sin usar", n)
	}
}

func TestReenviarVerificacionNoRevelaCuentasNiReenviaAActivas(t *testing.T) {
	env := nuevoEntorno(t)
	env.registrarYActivar("ana@example.com", clavePrueba)
	entregadosTrasAlta := env.correos.totalEntregados()

	c := env.cliente()
	activa := c.hacer(http.MethodPost, "/auth/reenviar-verificacion", map[string]string{"correo": "ana@example.com"})
	inexistente := c.hacer(http.MethodPost, "/auth/reenviar-verificacion", map[string]string{"correo": "nadie@example.com"})

	if activa.Estado != inexistente.Estado || activa.Crudo != inexistente.Crudo {
		t.Errorf("las respuestas permiten enumerar cuentas:\n%d %s\n%d %s",
			activa.Estado, activa.Crudo, inexistente.Estado, inexistente.Crudo)
	}
	if env.correos.totalEntregados() != entregadosTrasAlta {
		t.Error("no deberia reenviarse verificacion a una cuenta ya activa")
	}
	if n := env.contar(`SELECT count(*) FROM auditoria WHERE accion = 'verificacion.reenvio_innecesario'`); n != 1 {
		t.Errorf("el reenvio innecesario deberia auditarse, hay %d", n)
	}
}
