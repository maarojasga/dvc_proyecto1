package httpserver_test

import (
	"net/http"
	"testing"
)

// admin deja una cuenta de administrador con sesión iniciada.
func (e *entorno) admin(correo string) *cliente {
	e.t.Helper()
	e.registrarYVerificar(correo)
	e.asciendeA(correo, "admin")
	return e.entrar(correo, clavePrueba)
}

func TestSoloElAdministradorEntraALaGestion(t *testing.T) {
	env := nuevoEntorno(t)
	env.registrarYVerificar("estudiante@example.com")
	estudiante := env.entrar("estudiante@example.com", clavePrueba)

	rutas := []struct{ metodo, ruta string }{
		{http.MethodGet, "/admin/users"},
		{http.MethodGet, "/admin/audit"},
	}
	for _, r := range rutas {
		if res := estudiante.hacer(r.metodo, r.ruta, nil); res.Estado != http.StatusForbidden {
			t.Errorf("%s %s: un estudiante debería recibir 403, llegó %d", r.metodo, r.ruta, res.Estado)
		}
		if res := env.cliente().hacer(r.metodo, r.ruta, nil); res.Estado != http.StatusUnauthorized {
			t.Errorf("%s %s: sin sesión debería ser 401, llegó %d", r.metodo, r.ruta, res.Estado)
		}
	}
}

func TestLaBitacoraSeConsultaYRegistraLasAccionesAdministrativas(t *testing.T) {
	env := nuevoEntorno(t)
	admin := env.admin("admin@example.com")
	env.registrarYVerificar("victima@example.com")
	victimaID := env.consultarTexto(`SELECT id::text FROM users WHERE email='victima@example.com'`)

	if res := admin.hacer(http.MethodPatch, "/admin/users/"+victimaID+"/role",
		map[string]string{"role": "teacher"}); res.Estado != http.StatusOK {
		t.Fatalf("cambiar rol: %d %s", res.Estado, res.Crudo)
	}

	res := admin.hacer(http.MethodGet, "/admin/audit?action=user.role_updated", nil)
	if res.Estado != http.StatusOK {
		t.Fatalf("consultar auditoría: %d %s", res.Estado, res.Crudo)
	}
	items, _ := res.campo(t, "items").([]any)
	if len(items) != 1 {
		t.Fatalf("se esperaba 1 entrada del cambio de rol, hay %d: %s", len(items), res.Crudo)
	}
	entrada, _ := items[0].(map[string]any)
	if entrada["actor_email"] != "admin@example.com" {
		t.Errorf("la entrada debería identificar al administrador que actuó: %v", entrada["actor_email"])
	}
	if entrada["entity_id"] != victimaID {
		t.Errorf("la entrada debería apuntar a la cuenta afectada: %v", entrada["entity_id"])
	}
}

func TestSuspenderUnaCuentaCierraSusSesionesAlInstante(t *testing.T) {
	env := nuevoEntorno(t)
	admin := env.admin("admin@example.com")
	env.registrarYVerificar("estudiante@example.com")
	estudiante := env.entrar("estudiante@example.com", clavePrueba)
	estudianteID := env.consultarTexto(`SELECT id::text FROM users WHERE email='estudiante@example.com'`)

	if res := estudiante.hacer(http.MethodGet, "/auth/me", nil); res.Estado != http.StatusOK {
		t.Fatalf("el estudiante debería tener sesión: %d", res.Estado)
	}

	if res := admin.hacer(http.MethodPatch, "/admin/users/"+estudianteID+"/status",
		map[string]string{"status": "suspended"}); res.Estado != http.StatusOK {
		t.Fatalf("suspender: %d %s", res.Estado, res.Crudo)
	}

	// Suspender no basta con marcar la cuenta: la sesión abierta debe dejar de
	// valer en la siguiente petición.
	if res := estudiante.hacer(http.MethodGet, "/auth/me", nil); res.Estado != http.StatusUnauthorized {
		t.Errorf("la sesión del suspendido debería dejar de valer, llegó %d", res.Estado)
	}
	if n := env.contar(`SELECT count(*) FROM audit_logs WHERE action='user.status_updated'`); n != 1 {
		t.Errorf("la suspensión debería auditarse, hay %d entradas", n)
	}
}

func TestElAdministradorVeYRevocaLasSesionesDeUnaCuenta(t *testing.T) {
	env := nuevoEntorno(t)
	admin := env.admin("admin@example.com")
	env.registrarYVerificar("estudiante@example.com")
	movil := env.entrar("estudiante@example.com", clavePrueba)
	portatil := env.entrar("estudiante@example.com", clavePrueba)
	estudianteID := env.consultarTexto(`SELECT id::text FROM users WHERE email='estudiante@example.com'`)

	res := admin.hacer(http.MethodGet, "/admin/users/"+estudianteID+"/sessions", nil)
	if res.Estado != http.StatusOK {
		t.Fatalf("listar sesiones: %d %s", res.Estado, res.Crudo)
	}
	items, _ := res.campo(t, "items").([]any)
	if len(items) != 2 {
		t.Fatalf("se esperaban 2 sesiones, hay %d: %s", len(items), res.Crudo)
	}

	// Expulsión inmediata sin llegar a suspender la cuenta.
	if res := admin.hacer(http.MethodDelete, "/admin/users/"+estudianteID+"/sessions", nil); res.Estado != http.StatusNoContent {
		t.Fatalf("revocar sesiones: %d %s", res.Estado, res.Crudo)
	}
	for nombre, c := range map[string]*cliente{"móvil": movil, "portátil": portatil} {
		if r := c.hacer(http.MethodGet, "/auth/me", nil); r.Estado != http.StatusUnauthorized {
			t.Errorf("la sesión %s debería haberse cerrado, llegó %d", nombre, r.Estado)
		}
	}
	// La cuenta sigue activa: puede volver a entrar.
	env.entrar("estudiante@example.com", clavePrueba)
	if n := env.contar(`SELECT count(*) FROM audit_logs WHERE action='user.sessions_revoked'`); n != 1 {
		t.Errorf("la revocación debería auditarse, hay %d entradas", n)
	}
}

func TestSeProtegeAlUltimoAdministradorActivo(t *testing.T) {
	env := nuevoEntorno(t)
	admin := env.admin("admin@example.com")
	adminID := env.consultarTexto(`SELECT id::text FROM users WHERE email='admin@example.com'`)

	// Con un solo administrador, ni degradarlo ni suspenderlo.
	if res := admin.hacer(http.MethodPatch, "/admin/users/"+adminID+"/role",
		map[string]string{"role": "student"}); res.Estado != http.StatusConflict {
		t.Errorf("degradar al último administrador debería fallar, llegó %d: %s", res.Estado, res.Crudo)
	}
	if res := admin.hacer(http.MethodPatch, "/admin/users/"+adminID+"/status",
		map[string]string{"status": "suspended"}); res.Estado != http.StatusConflict {
		t.Errorf("suspender al último administrador debería fallar, llegó %d: %s", res.Estado, res.Crudo)
	}

	// Con un segundo administrador activo, la operación pasa a ser legítima.
	env.registrarYVerificar("admin2@example.com")
	env.asciendeA("admin2@example.com", "admin")
	if res := admin.hacer(http.MethodPatch, "/admin/users/"+adminID+"/role",
		map[string]string{"role": "student"}); res.Estado != http.StatusOK {
		t.Errorf("con dos administradores debería permitirse, llegó %d: %s", res.Estado, res.Crudo)
	}
}

func TestElAltaDeProfesorEsSoloPorAdministracion(t *testing.T) {
	env := nuevoEntorno(t)
	admin := env.admin("admin@example.com")

	res := admin.hacer(http.MethodPost, "/admin/teachers", map[string]string{
		"email": "profe@example.com", "full_name": "Profesora",
	})
	if res.Estado != http.StatusCreated {
		t.Fatalf("%d %s", res.Estado, res.Crudo)
	}
	if n := env.contar(`SELECT count(*) FROM users WHERE email='profe@example.com' AND role='teacher'`); n != 1 {
		t.Errorf("debería existir la cuenta de profesor, hay %d", n)
	}
	// Aquí sí se informa del duplicado: quien opera es un administrador de
	// confianza, no un formulario público.
	if repetida := admin.hacer(http.MethodPost, "/admin/teachers", map[string]string{
		"email": "profe@example.com", "full_name": "Otra",
	}); repetida.Estado != http.StatusConflict {
		t.Errorf("un alta duplicada debería informar al administrador, llegó %d", repetida.Estado)
	}
}
