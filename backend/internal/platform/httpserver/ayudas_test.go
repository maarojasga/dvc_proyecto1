package httpserver_test

import (
	"net/http"
	"strings"
)

// bytesContiene informa si el texto contiene la subcadena, evitando el caso
// degenerado de buscar una cadena vacia.
func bytesContiene(texto, subcadena string) bool {
	return subcadena != "" && strings.Contains(texto, subcadena)
}

// parsearCookies interpreta las cabeceras Set-Cookie de una respuesta.
func parsearCookies(cabeceras []string) []*http.Cookie {
	encabezado := http.Header{}
	for _, c := range cabeceras {
		encabezado.Add("Set-Cookie", c)
	}
	return (&http.Response{Header: encabezado}).Cookies()
}
