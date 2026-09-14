package httpserver_test

import (
	"net/http"
	"testing"
)

// Cubren la parte del alcance opcional 5.2 que pide "historial visible de
// revisiones".

// recursoDeTexto deja un recurso de texto listo para editar y devuelve la ruta
// base de su historial.
func (e *entorno) recursoDeTexto(c *cliente, versionID string) string {
	e.t.Helper()
	unidadID := e.unidadVacia(c, versionID)
	res := c.hacer(http.MethodPost, "/courses/versions/"+versionID+"/units/"+unidadID+"/resources",
		map[string]any{"type": "text", "title": "Lectura", "position": 1,
			"visible": true, "required": true, "text_content_md": "# Versión inicial"})
	if res.Estado != http.StatusCreated {
		e.t.Fatalf("crear recurso: %d %s", res.Estado, res.Crudo)
	}
	recursoID, _ := res.campo(e.t, "ID").(string)
	return "/courses/versions/" + versionID + "/resources/" + recursoID + "/revisions"
}

func TestCadaGuardadoDejaUnaRevisionAtribuida(t *testing.T) {
	env := nuevoEntorno(t)
	c, versionID := env.profesorConCurso("prof@example.com", "curso-hist")
	base := env.recursoDeTexto(c, versionID)

	for _, contenido := range []string{"# Primera", "# Segunda", "# Tercera"} {
		res := c.hacer(http.MethodPost, base, map[string]any{"content_md": contenido})
		if res.Estado != http.StatusCreated {
			t.Fatalf("guardar %q: %d %s", contenido, res.Estado, res.Crudo)
		}
	}

	res := c.hacer(http.MethodGet, base, nil)
	if res.Estado != http.StatusOK {
		t.Fatalf("listar: %d %s", res.Estado, res.Crudo)
	}
	items, _ := res.campo(t, "items").([]any)
	if len(items) != 3 {
		t.Fatalf("se esperaban 3 revisiones, hay %d: %s", len(items), res.Crudo)
	}

	// La más reciente primero, y numeradas de forma creciente.
	primera, _ := items[0].(map[string]any)
	if n, _ := primera["revision_number"].(float64); n != 3 {
		t.Errorf("la lista no empieza por la más reciente: %s", res.Crudo)
	}
	if correo, _ := primera["author_email"].(string); correo != "prof@example.com" {
		t.Errorf("la revisión no está atribuida: %s", res.Crudo)
	}

	// La lista no arrastra el contenido: un historial largo pesaría megabytes.
	if _, trae := primera["content_md"]; trae {
		t.Errorf("la lista no debería incluir el contenido de cada revisión: %s", res.Crudo)
	}
}

// El autoguardado dispara cada pocos segundos; sin esta guarda, media hora de
// escritura dejaría cientos de revisiones idénticas.
func TestGuardarLoMismoNoCreaUnaRevisionNueva(t *testing.T) {
	env := nuevoEntorno(t)
	c, versionID := env.profesorConCurso("prof@example.com", "curso-hist")
	base := env.recursoDeTexto(c, versionID)

	if res := c.hacer(http.MethodPost, base, map[string]any{"content_md": "# Igual"}); res.Estado != http.StatusCreated {
		t.Fatalf("primer guardado: %d %s", res.Estado, res.Crudo)
	}
	res := c.hacer(http.MethodPost, base, map[string]any{"content_md": "# Igual"})
	if res.Estado != http.StatusOK {
		t.Fatalf("repetir el guardado no es un error: %d %s", res.Estado, res.Crudo)
	}
	if estado, _ := res.campo(t, "status").(string); estado != "sin_cambios" {
		t.Errorf("estado inesperado: %s", res.Crudo)
	}

	lista := c.hacer(http.MethodGet, base, nil)
	if items, _ := lista.campo(t, "items").([]any); len(items) != 1 {
		t.Errorf("se guardó una revisión duplicada: %s", lista.Crudo)
	}
}

func TestUnaRevisionConcretaSeConsultaConSuContenido(t *testing.T) {
	env := nuevoEntorno(t)
	c, versionID := env.profesorConCurso("prof@example.com", "curso-hist")
	base := env.recursoDeTexto(c, versionID)

	c.hacer(http.MethodPost, base, map[string]any{"content_md": "# Uno"})
	c.hacer(http.MethodPost, base, map[string]any{"content_md": "# Dos"})

	res := c.hacer(http.MethodGet, base+"/1", nil)
	if res.Estado != http.StatusOK {
		t.Fatalf("ver revisión: %d %s", res.Estado, res.Crudo)
	}
	if contenido, _ := res.campo(t, "content_md").(string); contenido != "# Uno" {
		t.Errorf("contenido inesperado: %s", res.Crudo)
	}
}

// Restaurar no borra ni retrocede: deja una revisión nueva con el contenido
// antiguo, de modo que deshacer una restauración equivocada es otra
// restauración y no una pérdida.
func TestRestaurarDejaConstanciaEnLugarDeBorrar(t *testing.T) {
	env := nuevoEntorno(t)
	c, versionID := env.profesorConCurso("prof@example.com", "curso-hist")
	base := env.recursoDeTexto(c, versionID)

	c.hacer(http.MethodPost, base, map[string]any{"content_md": "# El bueno"})
	c.hacer(http.MethodPost, base, map[string]any{"content_md": "# Un desastre"})

	res := c.hacer(http.MethodPost, base+"/1/restore", nil)
	if res.Estado != http.StatusOK {
		t.Fatalf("restaurar: %d %s", res.Estado, res.Crudo)
	}
	if desde, _ := res.campo(t, "restored_from").(float64); desde != 1 {
		t.Errorf("no se indica de dónde se restauró: %s", res.Crudo)
	}

	// El historial conserva las tres: la original, el desastre y la
	// restauración.
	lista := c.hacer(http.MethodGet, base, nil)
	items, _ := lista.campo(t, "items").([]any)
	if len(items) != 3 {
		t.Fatalf("se esperaban 3 revisiones tras restaurar, hay %d: %s", len(items), lista.Crudo)
	}

	// Y el recurso vuelve a tener el contenido bueno.
	ultima := c.hacer(http.MethodGet, base+"/3", nil)
	if contenido, _ := ultima.campo(t, "content_md").(string); contenido != "# El bueno" {
		t.Errorf("la restauración no recuperó el contenido: %s", ultima.Crudo)
	}
}

func TestElHistorialDeUnCursoAjenoNoSeLeeNiSeEscribe(t *testing.T) {
	env := nuevoEntorno(t)
	c, versionID := env.profesorConCurso("prof@example.com", "curso-hist")
	base := env.recursoDeTexto(c, versionID)
	c.hacer(http.MethodPost, base, map[string]any{"content_md": "# Privado"})

	env.registrarYVerificar("otro@example.com")
	env.asciendeA("otro@example.com", "teacher")
	otro := env.entrar("otro@example.com", clavePrueba)

	if res := otro.hacer(http.MethodGet, base, nil); res.Estado == http.StatusOK {
		t.Errorf("un profesor ajeno no debería leer el historial, llegó %d", res.Estado)
	}
	if res := otro.hacer(http.MethodPost, base, map[string]any{"content_md": "# Intruso"}); res.Estado == http.StatusCreated {
		t.Errorf("un profesor ajeno no debería escribir en el historial, llegó %d", res.Estado)
	}
}

// El historial conserva quién escribió qué, que es lo que lo hace útil en un
// curso con más de un autor.
func TestElHistorialDistingueAutores(t *testing.T) {
	env := nuevoEntorno(t)
	admin := env.administrador("admin@example.com")
	c, versionID := env.profesorConCurso("prof@example.com", "curso-hist")
	base := env.recursoDeTexto(c, versionID)

	c.hacer(http.MethodPost, base, map[string]any{"content_md": "# Del profesor"})
	// La administración también puede editar, y queda con su propio nombre.
	if res := admin.hacer(http.MethodPost, base, map[string]any{"content_md": "# Revisado"}); res.Estado != http.StatusCreated {
		t.Fatalf("guardado de la administración: %d %s", res.Estado, res.Crudo)
	}

	lista := c.hacer(http.MethodGet, base, nil)
	if !contiene(lista.Crudo, "admin@example.com") || !contiene(lista.Crudo, "prof@example.com") {
		t.Errorf("el historial no distingue a los dos autores: %s", lista.Crudo)
	}
}
