package httpserver_test

import (
	"net/http"
	"testing"
)

// El contenido de un recurso de texto viaja en text_content_md y la URL
// externa en external_url. El frontend los mandaba con el nombre del modelo
// (TextContentMD, ExternalURL) y llegaban vacíos: Go empareja las claves JSON
// sin distinguir mayúsculas, pero no ignora los guiones bajos, así que title y
// type colaban y estos dos no. El recurso se creaba sin contenido y nadie se
// enteraba hasta abrirlo.
func TestElContenidoDeTextoSeGuardaYSeDevuelve(t *testing.T) {
	env := nuevoEntorno(t)
	c, versionID := env.profesorConCurso("prof-texto@example.com", "curso-texto")
	moduloID := moduloEnVersion(t, c, versionID, "Módulo")

	res := c.hacer(http.MethodPost, "/courses/versions/"+versionID+"/modules/"+moduloID+"/units",
		map[string]any{"title": "Unidad", "position": 1})
	unidadID, _ := res.campo(t, "ID").(string)

	const markdown = "# Introducción\n\nPrimer párrafo del material."
	res = c.hacer(http.MethodPost, "/courses/versions/"+versionID+"/units/"+unidadID+"/resources",
		map[string]any{
			"type": "text", "title": "Lectura", "position": 1, "visible": true,
			"text_content_md": markdown,
		})
	if res.Estado != http.StatusCreated {
		t.Fatalf("crear recurso: %d %s", res.Estado, res.Crudo)
	}
	if guardado, _ := res.campo(t, "TextContentMD").(string); guardado != markdown {
		t.Fatalf("al crear se guardó %q, esperaba %q", guardado, markdown)
	}
	recursoID, _ := res.campo(t, "ID").(string)

	// Y se recupera al releer la versión, que es de donde lo saca el editor.
	res = c.hacer(http.MethodGet, "/courses/versions/"+versionID, nil)
	if buscarContenidoDeRecurso(t, res, recursoID) != markdown {
		t.Fatalf("al releer la versión el contenido no coincide: %s", res.Crudo)
	}

	// El autoguardado del editor es este PATCH.
	const editado = "# Introducción\n\nPárrafo corregido.\n\n- un punto\n- otro"
	res = c.hacer(http.MethodPatch, "/courses/versions/"+versionID+"/resources/"+recursoID,
		map[string]any{
			"type": "text", "title": "Lectura", "position": 1, "visible": true,
			"text_content_md": editado,
		})
	if res.Estado != http.StatusOK {
		t.Fatalf("editar recurso: %d %s", res.Estado, res.Crudo)
	}
	res = c.hacer(http.MethodGet, "/courses/versions/"+versionID, nil)
	if guardado := buscarContenidoDeRecurso(t, res, recursoID); guardado != editado {
		t.Fatalf("tras el autoguardado quedó %q, esperaba %q", guardado, editado)
	}
}

func TestLaURLExternaSeGuarda(t *testing.T) {
	env := nuevoEntorno(t)
	c, versionID := env.profesorConCurso("prof-enlace@example.com", "curso-enlace")
	moduloID := moduloEnVersion(t, c, versionID, "Módulo")

	res := c.hacer(http.MethodPost, "/courses/versions/"+versionID+"/modules/"+moduloID+"/units",
		map[string]any{"title": "Unidad", "position": 1})
	unidadID, _ := res.campo(t, "ID").(string)

	const url = "https://ejemplo.edu/lectura-complementaria"
	res = c.hacer(http.MethodPost, "/courses/versions/"+versionID+"/units/"+unidadID+"/resources",
		map[string]any{
			"type": "link", "title": "Lectura extra", "position": 1, "visible": true,
			"external_url": url,
		})
	if res.Estado != http.StatusCreated {
		t.Fatalf("crear recurso: %d %s", res.Estado, res.Crudo)
	}
	if guardada, _ := res.campo(t, "ExternalURL").(string); guardada != url {
		t.Fatalf("se guardó %q, esperaba %q", guardada, url)
	}
}

// buscarRecurso recorre el árbol de la versión hasta el recurso pedido.
func buscarRecurso(t *testing.T, res respuesta, recursoID string) map[string]any {
	t.Helper()
	modulos, _ := res.Cuerpo["Modules"].([]any)
	for _, m := range modulos {
		mapaModulo, _ := m.(map[string]any)
		unidades, _ := mapaModulo["Units"].([]any)
		for _, u := range unidades {
			mapaUnidad, _ := u.(map[string]any)
			recursos, _ := mapaUnidad["Resources"].([]any)
			for _, rec := range recursos {
				mapaRecurso, _ := rec.(map[string]any)
				if id, _ := mapaRecurso["ID"].(string); id == recursoID {
					return mapaRecurso
				}
			}
		}
	}
	t.Fatalf("no se encontró el recurso %s en el árbol", recursoID)
	return nil
}

func buscarContenidoDeRecurso(t *testing.T, res respuesta, recursoID string) string {
	t.Helper()
	contenido, _ := buscarRecurso(t, res, recursoID)["TextContentMD"].(string)
	return contenido
}

// Editar los metadatos de un recurso no puede deshacer su carga. El UPDATE de
// autoría escribía processing_status y object_key desde la petición, así que
// cambiar el título de un vídeo ya subido lo dejaba sin archivo y sin estado.
func TestEditarUnRecursoNoDeshaceSuCarga(t *testing.T) {
	env := nuevoEntorno(t)
	c, versionID, recursoID := recursoBinario(t, env, "prof-no-pierde@example.com", "curso-no-pierde", "pdf")

	sha := env.almacen.pon([]byte("%PDF-1.7\nmaterial"))
	if res := subirPor(t, c, versionID, recursoID, map[string]any{"checksum_sha256": sha}); res.Estado != http.StatusOK {
		t.Fatalf("subir: %d %s", res.Estado, res.Crudo)
	}

	res := c.hacer(http.MethodPatch, "/courses/versions/"+versionID+"/resources/"+recursoID,
		map[string]any{"type": "pdf", "title": "Título corregido", "position": 1, "visible": true})
	if res.Estado != http.StatusOK {
		t.Fatalf("editar: %d %s", res.Estado, res.Crudo)
	}

	recurso := buscarRecurso(t, c.hacer(http.MethodGet, "/courses/versions/"+versionID, nil), recursoID)
	if titulo, _ := recurso["Title"].(string); titulo != "Título corregido" {
		t.Errorf("el título no se guardó: %v", recurso["Title"])
	}
	if clave, _ := recurso["ObjectKey"].(string); clave == "" {
		t.Error("editar el título dejó el recurso sin archivo (object_key vacío)")
	}
	if estado, _ := recurso["ProcessingStatus"].(string); estado != "ready" {
		t.Errorf("el estado de procesamiento quedó en %q, esperaba ready", estado)
	}
}
