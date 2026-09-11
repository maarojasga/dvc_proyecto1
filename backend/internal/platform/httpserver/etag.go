package httpserver

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"
)

// ETag y peticiones condicionales.
//
// El catálogo y el árbol de un curso publicado cambian poco y se piden mucho:
// cada vez que un estudiante vuelve a la lista, o navega entre recursos. Con
// ETag, el cliente reenvía lo que ya tiene y el servidor responde 304 sin
// cuerpo cuando nada cambió.
//
// El ETag se calcula del JSON que se iba a enviar. Es más trabajo que derivarlo
// de un updated_at, pero no se puede desincronizar: si la respuesta cambia, el
// ETag cambia, sin que haya que acordarse de tocar nada.

// writeJSONConETag escribe la respuesta con su ETag, y responde 304 si el
// cliente ya tiene esa misma versión.
//
// Solo tiene sentido en respuestas cacheables por el cliente: no se usa donde
// la respuesta lleva una URL firmada con caducidad, porque el cliente se
// quedaría con una URL vencida creyendo que sigue al día.
func writeJSONConETag(w http.ResponseWriter, r *http.Request, status int, v any) {
	cuerpo, err := json.Marshal(v)
	if err != nil {
		writeError(w, err)
		return
	}
	etag := etagDe(cuerpo)
	w.Header().Set("ETag", etag)
	// Vary por Cookie porque la misma ruta puede responder distinto según la
	// sesión: sin esto, una caché intermedia podría servirle a alguien la
	// respuesta de otro.
	w.Header().Add("Vary", "Cookie")

	if coincide(r.Header.Get("If-None-Match"), etag) {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Length", itoa(len(cuerpo)))
	w.WriteHeader(status)
	_, _ = w.Write(cuerpo)
}

// etagDe calcula el ETag del cuerpo. Fuerte (sin el prefijo W/), porque
// compara el byte exacto que se iba a enviar.
func etagDe(cuerpo []byte) string {
	suma := sha256.Sum256(cuerpo)
	// 128 bits bastan de sobra para distinguir versiones de una respuesta, y
	// dejan una cabecera corta.
	return `"` + base64.RawURLEncoding.EncodeToString(suma[:16]) + `"`
}

// coincide interpreta If-None-Match, que puede traer varios ETags separados
// por comas, o el comodín *.
func coincide(ifNoneMatch, etag string) bool {
	if ifNoneMatch == "" {
		return false
	}
	if strings.TrimSpace(ifNoneMatch) == "*" {
		return true
	}
	for _, candidato := range strings.Split(ifNoneMatch, ",") {
		candidato = strings.TrimSpace(candidato)
		// Un cliente puede devolver el ETag marcado como débil aunque se haya
		// emitido fuerte; para comparar igualdad de contenido da lo mismo.
		candidato = strings.TrimPrefix(candidato, "W/")
		if candidato == etag {
			return true
		}
	}
	return false
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}
