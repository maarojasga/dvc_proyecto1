package httpserver

import (
	"crypto/subtle"
	"net/http"
	"time"

	"github.com/DES-SOLUCIONES-CLOUD/proyecto-1/backend/internal/platform/security"
)

const (
	// CSRFCookieName lleva el token anti-CSRF. A diferencia de la cookie de
	// sesión no es httpOnly, a propósito: el frontend debe poder leerla para
	// repetirla en la cabecera.
	CSRFCookieName = "mooc_csrf"

	// CSRFHeaderName es donde el cliente repite el valor de la cookie.
	CSRFHeaderName = "X-CSRF-Token"
)

// issueCSRFCookie emite el token anti-CSRF junto con la sesión.
func (h *handlers) issueCSRFCookie(w http.ResponseWriter, expires time.Time) error {
	token, err := security.GenerateOpaqueToken()
	if err != nil {
		return err
	}
	http.SetCookie(w, &http.Cookie{
		Name:     CSRFCookieName,
		Value:    token,
		Path:     "/",
		Expires:  expires,
		HttpOnly: false,
		Secure:   h.deps.CookieSecure,
		SameSite: http.SameSiteLaxMode,
	})
	return nil
}

func (h *handlers) clearCSRFCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     CSRFCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: false,
		Secure:   h.deps.CookieSecure,
		SameSite: http.SameSiteLaxMode,
	})
}

func safeMethod(m string) bool {
	switch m {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return true
	}
	return false
}

// CSRF aplica la defensa de doble envío de cookie.
//
// SameSite=Lax ya bloquea la mayoría de los envíos entre sitios, pero no
// todos (una navegación de nivel superior con POST desde otro origen sí
// arrastra la cookie). Esta comprobación cubre el resto: toda petición que
// cambie estado autenticándose por cookie debe repetir en la cabecera el
// valor de la cookie CSRF, que un sitio atacante no puede leer.
//
// Solo se exige cuando la autenticación viene por cookie. Un cliente que use
// la cabecera Authorization no es vulnerable a CSRF, porque el navegador no
// la añade solo.
func CSRF(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if safeMethod(r.Method) {
			next.ServeHTTP(w, r)
			return
		}
		cookie, err := r.Cookie(SessionCookieName)
		if err != nil || cookie.Value == "" {
			next.ServeHTTP(w, r)
			return
		}
		enviado := r.Header.Get(CSRFHeaderName)
		esperado, err := r.Cookie(CSRFCookieName)
		if err != nil || esperado.Value == "" || enviado == "" ||
			subtle.ConstantTimeCompare([]byte(esperado.Value), []byte(enviado)) != 1 {
			writeError(w, ErrCSRF)
			return
		}
		next.ServeHTTP(w, r)
	})
}
