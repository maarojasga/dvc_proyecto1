package httpserver

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestElETagCambiaConElCuerpo(t *testing.T) {
	a := etagDe([]byte(`{"items":[1]}`))
	b := etagDe([]byte(`{"items":[2]}`))
	if a == b {
		t.Fatal("dos cuerpos distintos dieron el mismo ETag")
	}
	if a != etagDe([]byte(`{"items":[1]}`)) {
		t.Fatal("el ETag del mismo cuerpo cambió entre llamadas")
	}
	// Va entrecomillado, como exige la especificación de HTTP.
	if a[0] != '"' || a[len(a)-1] != '"' {
		t.Errorf("el ETag %s no va entrecomillado", a)
	}
}

func TestIfNoneMatch(t *testing.T) {
	etag := `"abc123"`
	casos := map[string]bool{
		"":                 false,
		`"abc123"`:         true,
		`"otro"`:           false,
		`"otro", "abc123"`: true,
		`W/"abc123"`:       true, // un cliente puede devolverlo como débil
		"*":                true,
		`"abc123x"`:        false,
	}
	for cabecera, esperado := range casos {
		if got := coincide(cabecera, etag); got != esperado {
			t.Errorf("coincide(%q, %s) = %v, esperaba %v", cabecera, etag, got, esperado)
		}
	}
}

func TestRespondeCon304CuandoNadaCambio(t *testing.T) {
	cuerpo := map[string]any{"items": []int{1, 2, 3}}

	// Primera petición: 200 con ETag.
	w1 := httptest.NewRecorder()
	r1 := httptest.NewRequest(http.MethodGet, "/catalogo", nil)
	writeJSONConETag(w1, r1, http.StatusOK, cuerpo)
	if w1.Code != http.StatusOK {
		t.Fatalf("primera petición: %d", w1.Code)
	}
	etag := w1.Header().Get("ETag")
	if etag == "" {
		t.Fatal("no se emitió ETag")
	}
	if w1.Body.Len() == 0 {
		t.Fatal("la primera respuesta llegó sin cuerpo")
	}

	// Segunda con el mismo ETag: 304 y sin cuerpo.
	w2 := httptest.NewRecorder()
	r2 := httptest.NewRequest(http.MethodGet, "/catalogo", nil)
	r2.Header.Set("If-None-Match", etag)
	writeJSONConETag(w2, r2, http.StatusOK, cuerpo)
	if w2.Code != http.StatusNotModified {
		t.Fatalf("con If-None-Match igual: %d, esperaba 304", w2.Code)
	}
	if w2.Body.Len() != 0 {
		t.Errorf("un 304 no lleva cuerpo, llegó %q", w2.Body.String())
	}
	// El ETag va también en el 304: el cliente lo necesita para seguir
	// preguntando.
	if w2.Header().Get("ETag") != etag {
		t.Error("el 304 no repite el ETag")
	}

	// Si el contenido cambia, vuelve el 200.
	w3 := httptest.NewRecorder()
	r3 := httptest.NewRequest(http.MethodGet, "/catalogo", nil)
	r3.Header.Set("If-None-Match", etag)
	writeJSONConETag(w3, r3, http.StatusOK, map[string]any{"items": []int{1, 2, 3, 4}})
	if w3.Code != http.StatusOK {
		t.Fatalf("con contenido nuevo: %d, esperaba 200", w3.Code)
	}
}

// Sin Vary por Cookie, una caché intermedia podría servir la respuesta de un
// usuario a otro.
func TestVaryPorCookie(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/catalogo", nil)
	writeJSONConETag(w, r, http.StatusOK, map[string]any{"a": 1})
	if got := w.Header().Get("Vary"); got == "" {
		t.Fatal("falta Vary")
	}
}
