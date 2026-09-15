package httpserver_test

import (
	"net/http"
	"testing"
)

// Cubren el alcance opcional 5.2: iframes restringidos mediante lista blanca,
// sandbox y política de permisos.
//
// La semilla de la migración no sobrevive a la limpieza del entorno —
// iframe_allowlist referencia a users, así que el TRUNCATE ... CASCADE se la
// lleva—, de modo que cada prueba registra el destino que necesita. Sale
// ganando: así se ejerce también el alta por administración.

// autorizarDestino registra un host en la lista blanca y devuelve su id.
func (e *entorno) autorizarDestino(admin *cliente, host, permisos string, subdominios bool) string {
	e.t.Helper()
	res := admin.hacer(http.MethodPost, "/admin/iframe-allowlist", map[string]any{
		"host": host, "include_subdomains": subdominios,
		"permissions": permisos, "description": "destino de prueba",
	})
	if res.Estado != http.StatusCreated {
		e.t.Fatalf("autorizar destino: %d %s", res.Estado, res.Crudo)
	}
	id, _ := res.campo(e.t, "id").(string)
	return id
}

// unidadVacia deja un módulo con una unidad y devuelve el id de la unidad.
// Se separa de la creación del recurso para que una prueba pueda intentar
// varios destinos sobre la misma estructura.
func (e *entorno) unidadVacia(c *cliente, versionID string) string {
	e.t.Helper()
	moduloID := moduloEnVersion(e.t, c, versionID, "Módulo")
	res := c.hacer(http.MethodPost, "/courses/versions/"+versionID+"/modules/"+moduloID+"/units",
		map[string]any{"title": "Unidad", "position": 1})
	unidadID, _ := res.campo(e.t, "ID").(string)
	return unidadID
}

// recursoIframe intenta crear un recurso incrustado y devuelve la respuesta
// cruda, para que la prueba decida si esperaba éxito o rechazo.
func (e *entorno) recursoIframe(c *cliente, versionID, unidadID, url string, posicion int) respuesta {
	e.t.Helper()
	return c.hacer(http.MethodPost, "/courses/versions/"+versionID+"/units/"+unidadID+"/resources",
		map[string]any{"type": "iframe", "title": "Simulador", "position": posicion,
			"visible": true, "external_url": url})
}

// cursoPublicadoConIframe deja publicado un curso cuyo único recurso es el
// iframe indicado. Hace falta publicar porque el contenido solo se entrega
// desde la versión vigente: en un borrador responde como inexistente.
func (e *entorno) cursoPublicadoConIframe(c *cliente, versionID, url string) string {
	e.t.Helper()
	unidadID := e.unidadVacia(c, versionID)
	creado := e.recursoIframe(c, versionID, unidadID, url, 1)
	if creado.Estado != http.StatusCreated {
		e.t.Fatalf("crear recurso: %d %s", creado.Estado, creado.Crudo)
	}
	recursoID, _ := creado.campo(e.t, "ID").(string)

	res := c.hacer(http.MethodPatch, "/courses/versions/"+versionID, map[string]any{
		"title": "Curso con simulador", "summary": "Resumen",
	})
	if res.Estado != http.StatusOK {
		e.t.Fatalf("metadatos: %d %s", res.Estado, res.Crudo)
	}
	if res := c.hacer(http.MethodPost, "/courses/versions/"+versionID+"/publish", nil); res.Estado != http.StatusOK {
		e.t.Fatalf("publicar: %d %s", res.Estado, res.Crudo)
	}
	return recursoID
}

func TestUnIframeFueraDeLaListaNoSePuedeGuardar(t *testing.T) {
	env := nuevoEntorno(t)
	admin := env.administrador("admin@example.com")
	env.autorizarDestino(admin, "www.geogebra.org", "fullscreen", false)

	c, versionID := env.profesorConCurso("prof@example.com", "curso-iframe")
	unidadID := env.unidadVacia(c, versionID)
	res := env.recursoIframe(c, versionID, unidadID, "https://sitio-cualquiera.com/embed", 1)

	if res.Estado != http.StatusUnprocessableEntity {
		t.Fatalf("se esperaba 422, llegó %d: %s", res.Estado, res.Crudo)
	}
	if codigo, _ := res.campo(t, "error", "code").(string); codigo != "iframe_host_not_allowed" {
		t.Errorf("código inesperado: %s", res.Crudo)
	}
}

// El control se hace sobre el host real, no sobre el parecido del texto.
func TestUnDominioQueImitaAlAutorizadoNoPasa(t *testing.T) {
	env := nuevoEntorno(t)
	admin := env.administrador("admin@example.com")
	env.autorizarDestino(admin, "vimeo.com", "fullscreen", true)

	c, versionID := env.profesorConCurso("prof@example.com", "curso-iframe")
	unidadID := env.unidadVacia(c, versionID)
	for i, url := range []string{
		"https://vimeo.com.atacante.net/x",
		"https://malicioso-vimeo.com/x",
	} {
		res := env.recursoIframe(c, versionID, unidadID, url, i+1)
		if res.Estado != http.StatusUnprocessableEntity {
			t.Errorf("%s no debería aceptarse, llegó %d: %s", url, res.Estado, res.Crudo)
		}
	}
}

func TestNoSeAdmiteHTTPEnUnIframe(t *testing.T) {
	env := nuevoEntorno(t)
	admin := env.administrador("admin@example.com")
	env.autorizarDestino(admin, "www.geogebra.org", "", false)

	c, versionID := env.profesorConCurso("prof@example.com", "curso-iframe")
	unidadID := env.unidadVacia(c, versionID)
	res := env.recursoIframe(c, versionID, unidadID, "http://www.geogebra.org/material/iframe/id/x", 1)
	if res.Estado != http.StatusUnprocessableEntity {
		t.Fatalf("se esperaba 422 por esquema, llegó %d: %s", res.Estado, res.Crudo)
	}
	if codigo, _ := res.campo(t, "error", "code").(string); codigo != "iframe_scheme_not_allowed" {
		t.Errorf("código inesperado: %s", res.Crudo)
	}
}

// El sandbox y la política de permisos los decide el servidor y viajan con el
// contenido: el cliente los aplica, no los elige.
func TestElContenidoDelIframeLlegaConSandboxYPermisos(t *testing.T) {
	env := nuevoEntorno(t)
	admin := env.administrador("admin@example.com")
	env.autorizarDestino(admin, "www.geogebra.org", "fullscreen", false)

	c, versionID := env.profesorConCurso("prof@example.com", "curso-iframe")
	recursoID := env.cursoPublicadoConIframe(c, versionID, "https://www.geogebra.org/material/iframe/id/abc")

	res := c.hacer(http.MethodGet, "/resources/"+recursoID+"/content", nil)
	if res.Estado != http.StatusOK {
		t.Fatalf("contenido: %d %s", res.Estado, res.Crudo)
	}
	sandbox, _ := res.campo(t, "sandbox").(string)
	if sandbox == "" {
		t.Fatalf("el contenido no trae sandbox: %s", res.Crudo)
	}
	if contiene(sandbox, "allow-top-navigation") || contiene(sandbox, "allow-downloads") {
		t.Errorf("el sandbox concede lo que no debe: %q", sandbox)
	}
	if permisos, _ := res.campo(t, "allow").(string); permisos != "fullscreen" {
		t.Errorf("la política de permisos no es la del destino: %s", res.Crudo)
	}
	if rp, _ := res.campo(t, "referrer_policy").(string); rp == "" {
		t.Errorf("falta la política de referente: %s", res.Crudo)
	}
}

// Quitar un destino de la lista corta la entrega del contenido ya publicado,
// sin tener que repasar los cursos uno por uno.
func TestQuitarUnDestinoCortaLaEntregaDeLoYaGuardado(t *testing.T) {
	env := nuevoEntorno(t)
	admin := env.administrador("admin@example.com")
	destinoID := env.autorizarDestino(admin, "www.geogebra.org", "fullscreen", false)

	c, versionID := env.profesorConCurso("prof@example.com", "curso-iframe")
	recursoID := env.cursoPublicadoConIframe(c, versionID, "https://www.geogebra.org/material/iframe/id/abc")

	if res := c.hacer(http.MethodGet, "/resources/"+recursoID+"/content", nil); res.Estado != http.StatusOK {
		t.Fatalf("el contenido debería entregarse antes de quitar el destino: %d", res.Estado)
	}

	if res := admin.hacer(http.MethodDelete, "/admin/iframe-allowlist/"+destinoID, nil); res.Estado != http.StatusNoContent {
		t.Fatalf("eliminar destino: %d %s", res.Estado, res.Crudo)
	}

	res := c.hacer(http.MethodGet, "/resources/"+recursoID+"/content", nil)
	if res.Estado != http.StatusUnprocessableEntity {
		t.Errorf("tras quitar el destino el contenido no debería entregarse, llegó %d: %s", res.Estado, res.Crudo)
	}

	// Y queda constancia de quién lo quitó.
	bitacora := admin.hacer(http.MethodGet, "/admin/audit", nil)
	if !contiene(bitacora.Crudo, "iframe_allowlist.removed") || !contiene(bitacora.Crudo, "www.geogebra.org") {
		t.Errorf("la baja del destino no quedó auditada con su host: %s", bitacora.Crudo)
	}
}

func TestSoloLaAdministracionCambiaLaListaBlanca(t *testing.T) {
	env := nuevoEntorno(t)
	c, _ := env.profesorConCurso("prof@example.com", "curso-iframe")

	res := c.hacer(http.MethodPost, "/admin/iframe-allowlist", map[string]any{"host": "atacante.com"})
	if res.Estado != http.StatusForbidden {
		t.Errorf("un profesor no debería poder ampliar la lista, llegó %d: %s", res.Estado, res.Crudo)
	}

	// Pero sí puede consultarla, que es lo que necesita para elegir destino.
	if lectura := c.hacer(http.MethodGet, "/iframe-allowlist", nil); lectura.Estado != http.StatusOK {
		t.Errorf("el profesor debería poder consultar la lista, llegó %d: %s", lectura.Estado, lectura.Crudo)
	}
}

func TestUnHostRepetidoActualizaSusPermisos(t *testing.T) {
	env := nuevoEntorno(t)
	admin := env.administrador("admin@example.com")
	env.autorizarDestino(admin, "vimeo.com", "fullscreen", false)
	env.autorizarDestino(admin, "vimeo.com", "fullscreen; autoplay", true)

	res := admin.hacer(http.MethodGet, "/iframe-allowlist", nil)
	items, _ := res.campo(t, "items").([]any)
	if len(items) != 1 {
		t.Fatalf("se esperaba una sola entrada para el host, hay %d: %s", len(items), res.Crudo)
	}
	entrada, _ := items[0].(map[string]any)
	if entrada["permissions"] != "fullscreen; autoplay" {
		t.Errorf("no se actualizaron los permisos: %s", res.Crudo)
	}
	if entrada["include_subdomains"] != true {
		t.Errorf("no se actualizó la regla de subdominios: %s", res.Crudo)
	}
}
