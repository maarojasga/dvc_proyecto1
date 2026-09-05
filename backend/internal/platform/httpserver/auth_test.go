package httpserver_test

import (
	"net/http"
	"testing"

	"github.com/DES-SOLUCIONES-CLOUD/proyecto-1/backend/internal/platform/httpserver"
)

const clavePrueba = "clave-de-prueba-1"

func TestRegistroCreaEstudiantePendienteYEnviaVerificacion(t *testing.T) {
	env := nuevoEntorno(t)
	c := env.cliente()

	res := c.hacer(http.MethodPost, "/auth/registro", map[string]string{
		"correo": "Ana@Example.com", "nombre": "Ana", "clave": clavePrueba,
	})
	if res.Estado != http.StatusAccepted {
		t.Fatalf("estado %d, cuerpo %s", res.Estado, res.Crudo)
	}

	// El correo se guarda normalizado, y la cuenta nace pendiente.
	if n := env.contar(`SELECT count(*) FROM usuarios WHERE correo = 'ana@example.com' AND estado = 'pendiente'`); n != 1 {
		t.Errorf("se esperaba 1 cuenta pendiente con el correo normalizado, hay %d", n)
	}

	// El registro publico solo produce estudiantes: los profesores y los
	// administradores se crean por administracion.
	if n := env.contar(`SELECT count(*) FROM usuario_roles WHERE rol = 'estudiante'`); n != 1 {
		t.Errorf("se esperaba el rol estudiante, hay %d filas", n)
	}
	if n := env.contar(`SELECT count(*) FROM usuario_roles WHERE rol IN ('profesor', 'administrador')`); n != 0 {
		t.Errorf("el registro publico no debe otorgar roles elevados, hay %d", n)
	}

	env.correos.tokenVerificacion(t, "ana@example.com")
}

func TestRegistroNoRevelaCuentasExistentes(t *testing.T) {
	env := nuevoEntorno(t)
	c := env.cliente()

	alta := map[string]string{"correo": "ana@example.com", "nombre": "Ana", "clave": clavePrueba}
	primera := c.hacer(http.MethodPost, "/auth/registro", alta)

	// Segundo intento con el mismo correo y otro nombre: debe responder
	// exactamente igual, sin pistas de que la cuenta ya existia.
	segunda := c.hacer(http.MethodPost, "/auth/registro", map[string]string{
		"correo": "ana@example.com", "nombre": "Impostor", "clave": "otra-clave-larga",
	})

	if primera.Estado != segunda.Estado || primera.Crudo != segunda.Crudo {
		t.Errorf("las respuestas difieren y permiten enumerar cuentas:\nprimera: %d %s\nsegunda: %d %s",
			primera.Estado, primera.Crudo, segunda.Estado, segunda.Crudo)
	}
	if n := env.contar(`SELECT count(*) FROM usuarios`); n != 1 {
		t.Errorf("no debe crearse una segunda cuenta, hay %d", n)
	}
	if n := env.contar(`SELECT count(*) FROM usuarios WHERE nombre = 'Impostor'`); n != 0 {
		t.Error("el segundo intento no debe sobrescribir los datos de la cuenta existente")
	}
	if n := env.contar(`SELECT count(*) FROM auditoria WHERE accion = 'registro.correo_duplicado'`); n != 1 {
		t.Errorf("el intento duplicado deberia quedar auditado, hay %d entradas", n)
	}
}

func TestRegistroValidaLosDatos(t *testing.T) {
	env := nuevoEntorno(t)
	c := env.cliente()

	casos := []struct {
		nombre string
		cuerpo map[string]string
		campo  string
	}{
		{"correo mal formado", map[string]string{"correo": "no-es-correo", "nombre": "Ana", "clave": clavePrueba}, "correo"},
		{"nombre vacio", map[string]string{"correo": "ana@example.com", "nombre": "  ", "clave": clavePrueba}, "nombre"},
		{"clave corta", map[string]string{"correo": "ana@example.com", "nombre": "Ana", "clave": "corta"}, "clave"},
	}
	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			res := c.hacer(http.MethodPost, "/auth/registro", caso.cuerpo)
			if res.Estado != http.StatusUnprocessableEntity {
				t.Fatalf("estado %d, cuerpo %s", res.Estado, res.Crudo)
			}
			if tipo := res.Cabecera.Get("Content-Type"); tipo != "application/problem+json; charset=utf-8" {
				t.Errorf("los errores deben ser problem details, llego %q", tipo)
			}
			errores, ok := res.campo(t, "errors").(map[string]any)
			if !ok || errores[caso.campo] == nil {
				t.Errorf("se esperaba un error en el campo %q, cuerpo %s", caso.campo, res.Crudo)
			}
		})
	}
	if n := env.contar(`SELECT count(*) FROM usuarios`); n != 0 {
		t.Errorf("ningun dato invalido deberia crear cuenta, hay %d", n)
	}
}

func TestVerificacionActivaLaCuentaYElTokenNoSeReutiliza(t *testing.T) {
	env := nuevoEntorno(t)
	c := env.cliente()

	c.hacer(http.MethodPost, "/auth/registro", map[string]string{
		"correo": "ana@example.com", "nombre": "Ana", "clave": clavePrueba,
	})
	token := env.correos.tokenVerificacion(t, "ana@example.com")

	res := c.hacer(http.MethodPost, "/auth/verificar-correo", map[string]string{"token": token})
	if res.Estado != http.StatusOK {
		t.Fatalf("estado %d, cuerpo %s", res.Estado, res.Crudo)
	}
	if n := env.contar(`SELECT count(*) FROM usuarios WHERE estado = 'activo' AND correo_verificado_en IS NOT NULL`); n != 1 {
		t.Error("la cuenta deberia quedar activa y con la fecha de verificacion")
	}

	// El token es de un solo uso.
	repetida := c.hacer(http.MethodPost, "/auth/verificar-correo", map[string]string{"token": token})
	if repetida.Estado != http.StatusBadRequest {
		t.Errorf("reusar el token deberia fallar, llego %d: %s", repetida.Estado, repetida.Crudo)
	}
}

func TestVerificacionRechazaTokenDesconocido(t *testing.T) {
	env := nuevoEntorno(t)
	res := env.cliente().hacer(http.MethodPost, "/auth/verificar-correo",
		map[string]string{"token": "token-que-nadie-emitio"})
	if res.Estado != http.StatusBadRequest {
		t.Errorf("estado %d, cuerpo %s", res.Estado, res.Crudo)
	}
}

func TestLoginRechazaCuentaSinVerificar(t *testing.T) {
	env := nuevoEntorno(t)
	c := env.cliente()
	c.hacer(http.MethodPost, "/auth/registro", map[string]string{
		"correo": "ana@example.com", "nombre": "Ana", "clave": clavePrueba,
	})

	res := c.hacer(http.MethodPost, "/auth/login", map[string]string{
		"correo": "ana@example.com", "clave": clavePrueba,
	})
	if res.Estado != http.StatusForbidden {
		t.Fatalf("estado %d, cuerpo %s", res.Estado, res.Crudo)
	}
	if _, hay := c.cookies[httpserver.CookieSesion]; hay {
		t.Error("no deberia emitirse cookie de sesion para una cuenta pendiente")
	}
}

func TestLoginNoDistingueCorreoInexistenteDeClaveErrada(t *testing.T) {
	env := nuevoEntorno(t)
	env.registrarYActivar("ana@example.com", clavePrueba)
	c := env.cliente()

	inexistente := c.hacer(http.MethodPost, "/auth/login", map[string]string{
		"correo": "nadie@example.com", "clave": clavePrueba,
	})
	claveErrada := c.hacer(http.MethodPost, "/auth/login", map[string]string{
		"correo": "ana@example.com", "clave": "clave-equivocada-1",
	})

	if inexistente.Estado != http.StatusUnauthorized || claveErrada.Estado != http.StatusUnauthorized {
		t.Fatalf("ambos deberian ser 401: %d y %d", inexistente.Estado, claveErrada.Estado)
	}
	if inexistente.Crudo != claveErrada.Crudo {
		t.Errorf("las respuestas permiten enumerar cuentas:\n%s\n%s", inexistente.Crudo, claveErrada.Crudo)
	}
	if n := env.contar(`SELECT count(*) FROM auditoria WHERE accion = 'sesion.intento_fallido'`); n != 2 {
		t.Errorf("ambos intentos deberian auditarse, hay %d", n)
	}
}

func TestLoginEmiteCookiesDefensivasYNuncaElTokenEnElCuerpo(t *testing.T) {
	env := nuevoEntorno(t)
	env.registrarYActivar("ana@example.com", clavePrueba)

	c := env.cliente()
	res := c.hacer(http.MethodPost, "/auth/login", map[string]string{
		"correo": "ana@example.com", "clave": clavePrueba,
	})
	if res.Estado != http.StatusOK {
		t.Fatalf("estado %d, cuerpo %s", res.Estado, res.Crudo)
	}

	tokenSesion := c.cookies[httpserver.CookieSesion]
	if tokenSesion == "" {
		t.Fatal("no se emitio la cookie de sesion")
	}
	if c.cookies[httpserver.CookieCSRF] == "" {
		t.Error("no se emitio la cookie anti-CSRF")
	}

	// El token de sesion no debe aparecer en el cuerpo: solo vive en la
	// cookie httpOnly, fuera del alcance de cualquier script.
	if bytesContiene(res.Crudo, tokenSesion) {
		t.Error("el token de sesion no debe viajar en el cuerpo de la respuesta")
	}

	var httpOnly, seguraLax bool
	for _, ck := range parsearCookies(res.Cabecera["Set-Cookie"]) {
		if ck.Name == httpserver.CookieSesion {
			httpOnly = ck.HttpOnly
			seguraLax = ck.SameSite == http.SameSiteLaxMode
		}
	}
	if !httpOnly {
		t.Error("la cookie de sesion deberia ser httpOnly")
	}
	if !seguraLax {
		t.Error("la cookie de sesion deberia ser SameSite=Lax")
	}

	// La sesion resuelve y devuelve el perfil.
	sesion := c.hacer(http.MethodGet, "/auth/sesion", nil)
	if sesion.Estado != http.StatusOK {
		t.Fatalf("sesion: estado %d, cuerpo %s", sesion.Estado, sesion.Crudo)
	}
	if correo := sesion.campo(t, "usuario", "correo"); correo != "ana@example.com" {
		t.Errorf("correo devuelto: %v", correo)
	}
	if hash := sesion.campo(t, "usuario", "claveHash"); hash != nil {
		t.Error("el hash de la clave nunca debe salir en la respuesta")
	}
}

func TestSesionRequiereAutenticacion(t *testing.T) {
	env := nuevoEntorno(t)
	res := env.cliente().hacer(http.MethodGet, "/auth/sesion", nil)
	if res.Estado != http.StatusUnauthorized {
		t.Errorf("estado %d, cuerpo %s", res.Estado, res.Crudo)
	}
}

func TestReintentarElAltaReenviaLaVerificacionSiSiguePendiente(t *testing.T) {
	env := nuevoEntorno(t)
	c := env.cliente()
	alta := map[string]string{"correo": "ana@example.com", "nombre": "Ana", "clave": clavePrueba}

	c.hacer(http.MethodPost, "/auth/registro", alta)
	primerToken := env.correos.tokenVerificacion(t, "ana@example.com")

	// Si el primer correo se pierde (SMTP caido, buzon lleno), el usuario
	// vuelve a enviar el formulario. La cuenta ya existe, asi que sin reenvio
	// se quedaria pendiente para siempre: el alta responderia 202 sin emitir
	// nada nunca mas.
	res := c.hacer(http.MethodPost, "/auth/registro", alta)
	if res.Estado != http.StatusAccepted {
		t.Fatalf("estado %d, cuerpo %s", res.Estado, res.Crudo)
	}
	segundoToken := env.correos.tokenVerificacion(t, "ana@example.com")
	if segundoToken == primerToken {
		t.Fatal("el reintento deberia emitir un token nuevo")
	}

	// Y el token nuevo activa la cuenta.
	verificacion := c.hacer(http.MethodPost, "/auth/verificar-correo", map[string]string{"token": segundoToken})
	if verificacion.Estado != http.StatusOK {
		t.Fatalf("verificacion: estado %d, cuerpo %s", verificacion.Estado, verificacion.Crudo)
	}
	if n := env.contar(`SELECT count(*) FROM usuarios`); n != 1 {
		t.Errorf("el reintento no debe crear una segunda cuenta, hay %d", n)
	}
}

func TestReintentarElAltaSobreCuentaActivaNoEmiteNada(t *testing.T) {
	env := nuevoEntorno(t)
	env.registrarYActivar("ana@example.com", clavePrueba)
	entregadosTrasAlta := env.correos.totalEntregados()

	res := env.cliente().hacer(http.MethodPost, "/auth/registro", map[string]string{
		"correo": "ana@example.com", "nombre": "Impostor", "clave": "otra-clave-larga",
	})
	if res.Estado != http.StatusAccepted {
		t.Fatalf("estado %d, cuerpo %s", res.Estado, res.Crudo)
	}
	// Una cuenta ya activa no debe recibir correos por que un tercero teclee
	// su direccion en el formulario de alta.
	if env.correos.totalEntregados() != entregadosTrasAlta {
		t.Error("no deberia enviarse correo por reintentar el alta de una cuenta activa")
	}
}
