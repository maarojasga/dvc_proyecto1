package httpserver

import (
	"net/http"
	"time"
)

const (
	// CookieSesion lleva el token de sesion. Es httpOnly: el JavaScript de la
	// pagina no puede leerla, asi que un XSS no puede exfiltrar la sesion.
	CookieSesion = "mooc_sesion"

	// CookieCSRF lleva el token anti-CSRF. No es httpOnly a proposito: el
	// frontend debe leerla para reenviarla en la cabecera X-CSRF-Token.
	CookieCSRF = "mooc_csrf"

	// CabeceraCSRF es donde el cliente repite el token de la cookie.
	CabeceraCSRF = "X-CSRF-Token"
)

// fijarCookieSesion emite la cookie de sesion.
func (s *Servidor) fijarCookieSesion(w http.ResponseWriter, token string, expira time.Time) {
	http.SetCookie(w, &http.Cookie{
		Name:     CookieSesion,
		Value:    token,
		Path:     "/",
		Expires:  expira,
		HttpOnly: true,
		Secure:   s.cookieSegura,
		// Lax deja pasar la navegacion normal desde enlaces externos y a la
		// vez impide que un formulario de otro sitio envie la cookie.
		SameSite: http.SameSiteLaxMode,
	})
}

// fijarCookieCSRF emite el token anti-CSRF que acompana a la sesion.
func (s *Servidor) fijarCookieCSRF(w http.ResponseWriter, token string, expira time.Time) {
	http.SetCookie(w, &http.Cookie{
		Name:     CookieCSRF,
		Value:    token,
		Path:     "/",
		Expires:  expira,
		HttpOnly: false,
		Secure:   s.cookieSegura,
		SameSite: http.SameSiteLaxMode,
	})
}

// borrarCookies invalida ambas cookies en el navegador.
func (s *Servidor) borrarCookies(w http.ResponseWriter) {
	for _, nombre := range []string{CookieSesion, CookieCSRF} {
		http.SetCookie(w, &http.Cookie{
			Name:     nombre,
			Value:    "",
			Path:     "/",
			MaxAge:   -1,
			HttpOnly: nombre == CookieSesion,
			Secure:   s.cookieSegura,
			SameSite: http.SameSiteLaxMode,
		})
	}
}

// tokenDeCookie devuelve el valor de una cookie, o cadena vacia.
func tokenDeCookie(r *http.Request, nombre string) string {
	c, err := r.Cookie(nombre)
	if err != nil {
		return ""
	}
	return c.Value
}
