package httpserver_test

import (
	"net/http"
	"testing"

	"github.com/DES-SOLUCIONES-CLOUD/proyecto-1/backend/internal/platform/httpserver"
)

func TestRegistroCreaEstudiantePendienteYEnviaVerificacion(t *testing.T) {
	env := nuevoEntorno(t)

	res := env.cliente().hacer(http.MethodPost, "/auth/register", map[string]string{
		"email": "ana@example.com", "password": clavePrueba, "full_name": "Ana",
	})
	if res.Estado != http.StatusAccepted {
		t.Fatalf("%d %s", res.Estado, res.Crudo)
	}

	// El registro público solo produce estudiantes pendientes: los profesores
	// y administradores se crean por administración.
	if n := env.contar(`SELECT count(*) FROM users WHERE email='ana@example.com' AND role='student' AND status='pending_verification'`); n != 1 {
		t.Errorf("se esperaba 1 estudiante pendiente, hay %d", n)
	}
	if n := env.contar(`SELECT count(*) FROM users WHERE role IN ('teacher','admin')`); n != 0 {
		t.Errorf("el registro público no debe otorgar roles elevados, hay %d", n)
	}
	env.correos.tokenPara(t, "ana@example.com")

	if n := env.contar(`SELECT count(*) FROM audit_logs WHERE action='auth.register.created'`); n != 1 {
		t.Errorf("el alta debería auditarse, hay %d entradas", n)
	}
}

func TestRegistroNoRevelaCuentasExistentes(t *testing.T) {
	env := nuevoEntorno(t)
	c := env.cliente()

	primera := c.hacer(http.MethodPost, "/auth/register", map[string]string{
		"email": "ana@example.com", "password": clavePrueba, "full_name": "Ana",
	})
	segunda := c.hacer(http.MethodPost, "/auth/register", map[string]string{
		"email": "ana@example.com", "password": "Otra-Clave-Larga-9", "full_name": "Impostor",
	})

	if primera.Estado != segunda.Estado || primera.Crudo != segunda.Crudo {
		t.Errorf("las respuestas difieren y permiten enumerar cuentas:\n%d %s\n%d %s",
			primera.Estado, primera.Crudo, segunda.Estado, segunda.Crudo)
	}
	if n := env.contar(`SELECT count(*) FROM users`); n != 1 {
		t.Errorf("no debe crearse una segunda cuenta, hay %d", n)
	}
	if n := env.contar(`SELECT count(*) FROM users WHERE full_name='Impostor'`); n != 0 {
		t.Error("el segundo intento no debe sobrescribir la cuenta existente")
	}
}

func TestReintentarElAltaReenviaLaVerificacionSiSiguePendiente(t *testing.T) {
	env := nuevoEntorno(t)
	c := env.cliente()
	alta := map[string]string{"email": "ana@example.com", "password": clavePrueba, "full_name": "Ana"}

	c.hacer(http.MethodPost, "/auth/register", alta)
	primerToken := env.correos.tokenPara(t, "ana@example.com")

	// Si el primer correo se pierde, el usuario reenvía el formulario. Sin
	// reenvío la cuenta quedaría pendiente para siempre: el alta respondería
	// 202 sin volver a emitir nada nunca.
	if res := c.hacer(http.MethodPost, "/auth/register", alta); res.Estado != http.StatusAccepted {
		t.Fatalf("%d %s", res.Estado, res.Crudo)
	}
	segundoToken := env.correos.tokenPara(t, "ana@example.com")
	if segundoToken == primerToken {
		t.Fatal("el reintento debería emitir un token nuevo")
	}
	if res := c.hacer(http.MethodPost, "/auth/verify-email", map[string]string{"token": segundoToken}); res.Estado != http.StatusOK {
		t.Fatalf("verificación: %d %s", res.Estado, res.Crudo)
	}
}

func TestReintentarElAltaSobreCuentaActivaNoEnviaCorreo(t *testing.T) {
	env := nuevoEntorno(t)
	env.registrarYVerificar("ana@example.com")
	entregados := env.correos.total()

	res := env.cliente().hacer(http.MethodPost, "/auth/register", map[string]string{
		"email": "ana@example.com", "password": "Otra-Clave-Larga-9", "full_name": "Impostor",
	})
	if res.Estado != http.StatusAccepted {
		t.Fatalf("%d %s", res.Estado, res.Crudo)
	}
	// Una cuenta activa no debe recibir correos porque un tercero teclee su
	// dirección en el formulario de alta.
	if env.correos.total() != entregados {
		t.Error("no debería enviarse correo por reintentar el alta de una cuenta activa")
	}
}

func TestVerificacionActivaLaCuentaYElTokenNoSeReutiliza(t *testing.T) {
	env := nuevoEntorno(t)
	c := env.cliente()
	c.hacer(http.MethodPost, "/auth/register", map[string]string{
		"email": "ana@example.com", "password": clavePrueba, "full_name": "Ana",
	})
	token := env.correos.tokenPara(t, "ana@example.com")

	if res := c.hacer(http.MethodPost, "/auth/verify-email", map[string]string{"token": token}); res.Estado != http.StatusOK {
		t.Fatalf("%d %s", res.Estado, res.Crudo)
	}
	if n := env.contar(`SELECT count(*) FROM users WHERE status='active' AND email_verified_at IS NOT NULL`); n != 1 {
		t.Error("la cuenta debería quedar activa y con fecha de verificación")
	}
	if res := c.hacer(http.MethodPost, "/auth/verify-email", map[string]string{"token": token}); res.Estado != http.StatusBadRequest {
		t.Errorf("reusar el token debería fallar, llegó %d: %s", res.Estado, res.Crudo)
	}
}

func TestReenviarVerificacionNoRevelaCuentasNiReenviaAActivas(t *testing.T) {
	env := nuevoEntorno(t)
	env.registrarYVerificar("ana@example.com")
	entregados := env.correos.total()
	c := env.cliente()

	activa := c.hacer(http.MethodPost, "/auth/resend-verification", map[string]string{"email": "ana@example.com"})
	inexistente := c.hacer(http.MethodPost, "/auth/resend-verification", map[string]string{"email": "nadie@example.com"})

	if activa.Estado != inexistente.Estado || activa.Crudo != inexistente.Crudo {
		t.Errorf("las respuestas permiten enumerar cuentas:\n%d %s\n%d %s",
			activa.Estado, activa.Crudo, inexistente.Estado, inexistente.Crudo)
	}
	if env.correos.total() != entregados {
		t.Error("no debería reenviarse verificación a una cuenta ya activa")
	}
}

func TestLoginRechazaCuentaSinVerificar(t *testing.T) {
	env := nuevoEntorno(t)
	c := env.cliente()
	c.hacer(http.MethodPost, "/auth/register", map[string]string{
		"email": "ana@example.com", "password": clavePrueba, "full_name": "Ana",
	})

	res := c.hacer(http.MethodPost, "/auth/login", map[string]string{
		"email": "ana@example.com", "password": clavePrueba,
	})
	if res.Estado != http.StatusForbidden {
		t.Fatalf("%d %s", res.Estado, res.Crudo)
	}
	if _, hay := c.cookies[httpserver.SessionCookieName]; hay {
		t.Error("no debería emitirse cookie de sesión para una cuenta pendiente")
	}
}

func TestLoginNoDistingueCorreoInexistenteDeClaveErrada(t *testing.T) {
	env := nuevoEntorno(t)
	env.registrarYVerificar("ana@example.com")
	c := env.cliente()

	inexistente := c.hacer(http.MethodPost, "/auth/login", map[string]string{
		"email": "nadie@example.com", "password": clavePrueba,
	})
	claveErrada := c.hacer(http.MethodPost, "/auth/login", map[string]string{
		"email": "ana@example.com", "password": "Clave-Equivocada-1",
	})

	if inexistente.Estado != http.StatusUnauthorized || claveErrada.Estado != http.StatusUnauthorized {
		t.Fatalf("ambos deberían ser 401: %d y %d", inexistente.Estado, claveErrada.Estado)
	}
	if inexistente.Crudo != claveErrada.Crudo {
		t.Errorf("las respuestas permiten enumerar cuentas:\n%s\n%s", inexistente.Crudo, claveErrada.Crudo)
	}
	if n := env.contar(`SELECT count(*) FROM audit_logs WHERE action='auth.login.failed'`); n != 2 {
		t.Errorf("ambos intentos deberían auditarse, hay %d", n)
	}
}

func TestLoginEmiteCookiesDefensivasYNuncaElTokenEnElCuerpo(t *testing.T) {
	env := nuevoEntorno(t)
	env.registrarYVerificar("ana@example.com")

	c := env.cliente()
	res := c.hacer(http.MethodPost, "/auth/login", map[string]string{
		"email": "ana@example.com", "password": clavePrueba,
	})
	if res.Estado != http.StatusOK {
		t.Fatalf("%d %s", res.Estado, res.Crudo)
	}

	tokenSesion := c.cookies[httpserver.SessionCookieName]
	if tokenSesion == "" {
		t.Fatal("no se emitió la cookie de sesión")
	}
	if c.cookies[httpserver.CSRFCookieName] == "" {
		t.Error("no se emitió la cookie anti-CSRF")
	}
	// El token de sesión no debe salir en el cuerpo: solo vive en la cookie
	// httpOnly, fuera del alcance de cualquier script.
	if contiene(res.Crudo, tokenSesion) {
		t.Error("el token de sesión no debe viajar en el cuerpo de la respuesta")
	}

	var httpOnly, sameSiteLax bool
	for _, ck := range cookiesDe(res.Cabecera["Set-Cookie"]) {
		if ck.Name == httpserver.SessionCookieName {
			httpOnly = ck.HttpOnly
			sameSiteLax = ck.SameSite == http.SameSiteLaxMode
		}
	}
	if !httpOnly {
		t.Error("la cookie de sesión debería ser httpOnly")
	}
	if !sameSiteLax {
		t.Error("la cookie de sesión debería ser SameSite=Lax")
	}

	if me := c.hacer(http.MethodGet, "/auth/me", nil); me.Estado != http.StatusOK {
		t.Fatalf("me: %d %s", me.Estado, me.Crudo)
	}
}
