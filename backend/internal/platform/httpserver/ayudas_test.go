package httpserver_test

import (
	"net/http"
	"strings"
)

// contiene evita el caso degenerado de buscar una subcadena vacía.
func contiene(texto, subcadena string) bool {
	return subcadena != "" && strings.Contains(texto, subcadena)
}

// cookiesDe interpreta las cabeceras Set-Cookie de una respuesta.
func cookiesDe(cabeceras []string) []*http.Cookie {
	h := http.Header{}
	for _, c := range cabeceras {
		h.Add("Set-Cookie", c)
	}
	return (&http.Response{Header: h}).Cookies()
}
