package httpserver_test

import (
	"fmt"
	"net/http"
	"testing"
)

// cursoPublicado deja un curso publicado y devuelve su id.
func cursoPublicado(t *testing.T, env *entorno, c *cliente, slug, titulo string) string {
	t.Helper()
	res := c.hacer(http.MethodPost, "/courses", map[string]any{"slug": slug, "title": titulo})
	if res.Estado != http.StatusCreated {
		t.Fatalf("crear curso %s: %d %s", slug, res.Estado, res.Crudo)
	}
	cursoID, _ := res.campo(t, "course_id").(string)
	versionID, _ := res.campo(t, "version_id").(string)

	if res := c.hacer(http.MethodPatch, "/courses/versions/"+versionID, map[string]any{
		"title": titulo, "summary": "Resumen de " + titulo, "description_md": "# " + titulo,
		"category": "Cloud", "level": "basico", "language": "es",
		"approval_min_score": 60, "approval_required_resources_pct": 100,
	}); res.Estado != http.StatusOK {
		t.Fatalf("metadatos: %d %s", res.Estado, res.Crudo)
	}

	moduloID := moduloEnVersion(t, c, versionID, "Módulo")
	res = c.hacer(http.MethodPost, "/courses/versions/"+versionID+"/modules/"+moduloID+"/units",
		map[string]any{"title": "Unidad", "position": 1})
	unidadID, _ := res.campo(t, "ID").(string)
	if res := c.hacer(http.MethodPost, "/courses/versions/"+versionID+"/units/"+unidadID+"/resources",
		map[string]any{"type": "text", "title": "Lectura", "position": 1, "visible": true,
			"text_content_md": "# Contenido"}); res.Estado != http.StatusCreated {
		t.Fatalf("recurso: %d %s", res.Estado, res.Crudo)
	}
	if res := c.hacer(http.MethodPost, "/courses/versions/"+versionID+"/publish", nil); res.Estado != http.StatusOK {
		t.Fatalf("publicar %s: %d %s", slug, res.Estado, res.Crudo)
	}
	return cursoID
}

// La paginación por cursor tiene que recorrer todo el catálogo exactamente una
// vez. Con OFFSET, publicar un curso a mitad del recorrido desplaza las filas y
// el lector se salta una o ve otra dos veces; con cursor, no.
func TestElCatalogoSePaginaConCursorSinRepetirNiSaltarse(t *testing.T) {
	env := nuevoEntorno(t)
	prof := env.profesor("prof-catalogo@example.com")
	const total = 5
	for i := 0; i < total; i++ {
		cursoPublicado(t, env, prof, fmt.Sprintf("curso-%d", i), fmt.Sprintf("Curso %d", i))
	}

	anonimo := env.cliente()
	vistos := map[string]bool{}
	cursor := ""
	for vueltas := 0; ; vueltas++ {
		if vueltas > total+2 {
			t.Fatal("la paginación no terminó; hay un ciclo")
		}
		ruta := "/catalog?limit=2"
		if cursor != "" {
			ruta += "&cursor=" + cursor
		}
		res := anonimo.hacer(http.MethodGet, ruta, nil)
		if res.Estado != http.StatusOK {
			t.Fatalf("catálogo: %d %s", res.Estado, res.Crudo)
		}
		items, _ := res.Cuerpo["items"].([]any)
		if len(items) > 2 {
			t.Fatalf("se pidieron 2 por página y llegaron %d", len(items))
		}
		for _, it := range items {
			m, _ := it.(map[string]any)
			id, _ := m["ID"].(string)
			if vistos[id] {
				t.Errorf("la versión %s salió dos veces", id)
			}
			vistos[id] = true
		}
		siguiente, _ := res.Cuerpo["next_cursor"].(string)

		// A mitad del recorrido se publica un curso más. Con OFFSET esto
		// desplazaría las filas y haría que se repitiera o se saltara alguna.
		if vueltas == 1 {
			cursoPublicado(t, env, prof, "curso-intruso", "Curso intruso")
		}

		if siguiente == "" {
			break
		}
		cursor = siguiente
	}

	if len(vistos) < total {
		t.Fatalf("se vieron %d versiones de las %d que había al empezar", len(vistos), total)
	}
}

// Un cursor corrupto es una petición inválida. Interpretarlo como el principio
// devolvería la primera página en silencio y el cliente creería avanzar.
func TestUnCursorInvalidoSeRechaza(t *testing.T) {
	env := nuevoEntorno(t)
	res := env.cliente().hacer(http.MethodGet, "/catalog?cursor=esto-no-es-un-cursor", nil)
	if res.Estado != http.StatusBadRequest {
		t.Fatalf("esperaba 400, obtuve %d: %s", res.Estado, res.Crudo)
	}
}

// El catálogo y el árbol de un curso se piden en cada vuelta a la lista, así
// que responden con ETag y con 304 cuando nada cambió.
func TestElCatalogoRespondeCon304SiNadaCambio(t *testing.T) {
	env := nuevoEntorno(t)
	prof := env.profesor("prof-etag@example.com")
	cursoID := cursoPublicado(t, env, prof, "curso-etag", "Curso con ETag")

	for _, ruta := range []string{"/catalog", "/catalog/" + cursoID} {
		anonimo := env.cliente()
		primera := anonimo.hacer(http.MethodGet, ruta, nil)
		if primera.Estado != http.StatusOK {
			t.Fatalf("%s: %d %s", ruta, primera.Estado, primera.Crudo)
		}
		etag := primera.Cabecera.Get("ETag")
		if etag == "" {
			t.Fatalf("%s no emitió ETag", ruta)
		}

		segunda := anonimo.hacer(http.MethodGet, ruta, nil, [2]string{"If-None-Match", etag})
		if segunda.Estado != http.StatusNotModified {
			t.Errorf("%s con If-None-Match: %d, esperaba 304 (cuerpo %s)", ruta, segunda.Estado, segunda.Crudo)
		}
		if segunda.Crudo != "" {
			t.Errorf("%s devolvió cuerpo en el 304: %q", ruta, segunda.Crudo)
		}
	}
}

// Publicar otro curso cambia el catálogo, así que el ETag deja de valer: si no,
// el estudiante no vería el curso nuevo.
func TestElETagDelCatalogoCambiaAlPublicar(t *testing.T) {
	env := nuevoEntorno(t)
	prof := env.profesor("prof-etag2@example.com")
	cursoPublicado(t, env, prof, "curso-uno", "Curso uno")

	anonimo := env.cliente()
	antes := anonimo.hacer(http.MethodGet, "/catalog", nil)
	etag := antes.Cabecera.Get("ETag")

	cursoPublicado(t, env, prof, "curso-dos", "Curso dos")

	despues := anonimo.hacer(http.MethodGet, "/catalog", nil, [2]string{"If-None-Match", etag})
	if despues.Estado != http.StatusOK {
		t.Fatalf("tras publicar otro curso el catálogo respondió %d; el ETag no cambió", despues.Estado)
	}
}
