package httpserver

import (
	"context"
	"errors"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"github.com/DES-SOLUCIONES-CLOUD/proyecto-1/backend/internal/app/auth"
	"github.com/DES-SOLUCIONES-CLOUD/proyecto-1/backend/internal/domain/user"
)

var (
	ErrUnauthenticated = errors.New("httpserver: se requiere autenticación")
	ErrForbidden       = errors.New("httpserver: no tienes permiso para esta operación")
	ErrBadRequest      = errors.New("httpserver: solicitud inválida")
	ErrCSRF            = errors.New("httpserver: falta o no coincide el token anti-CSRF")

	// ErrIdempotencyInFlight indica que otra petición con la misma
	// Idempotency-Key sigue ejecutándose.
	ErrIdempotencyInFlight = errors.New("httpserver: ya hay una petición idéntica en curso")

	// ErrColaNoDisponible indica que la instancia no tiene inspector de cola
	// configurado, así que no puede informar de su estado.
	ErrColaNoDisponible = errors.New("httpserver: la inspección de la cola no está disponible")
)

type ctxKey int

const (
	ctxUser ctxKey = iota
	ctxRequestID
	ctxKeySession
)

// SessionCookieName es el nombre de la cookie httpOnly que transporta el
// token de sesión opaco cuando el cliente es un navegador.
const SessionCookieName = "mooc_session"

func withUser(ctx context.Context, u *user.User) context.Context {
	return context.WithValue(ctx, ctxUser, u)
}

func withSession(ctx context.Context, s *user.Session) context.Context {
	return context.WithValue(ctx, ctxKeySession, s)
}

// SessionFromContext expone la sesión en curso a los handlers, que la
// necesitan para saber cuál de las sesiones listadas es la actual y para no
// cerrarla al revocar las demás.
func SessionFromContext(ctx context.Context) (*user.Session, bool) {
	s, ok := ctx.Value(ctxKeySession).(*user.Session)
	return s, ok
}

// UserFromContext expone el usuario autenticado a los handlers.
func UserFromContext(ctx context.Context) (*user.User, bool) {
	u, ok := ctx.Value(ctxUser).(*user.User)
	return u, ok
}

func requestIDFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(ctxRequestID).(string); ok {
		return v
	}
	return ""
}

func extractToken(r *http.Request) string {
	if h := r.Header.Get("Authorization"); strings.HasPrefix(h, "Bearer ") {
		return strings.TrimPrefix(h, "Bearer ")
	}
	if c, err := r.Cookie(SessionCookieName); err == nil {
		return c.Value
	}
	return ""
}

// RequestID asigna un identificador único por solicitud, propagado en la
// respuesta para correlacionar logs, trazas y evidencia de aceptación.
func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := uuid.New().String()
		w.Header().Set("X-Request-Id", id)
		ctx := context.WithValue(r.Context(), ctxRequestID, id)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// Recover convierte pánicos en una respuesta 500 uniforme en lugar de tumbar
// el proceso o filtrar trazas internas al cliente.
func Recover(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				log.Printf("panic recuperado [%s]: %v", requestIDFromContext(r.Context()), rec)
				writeError(w, errors.New("internal"))
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// Logging deja un registro estructurado mínimo por solicitud (método, ruta,
// estado, duración) correlacionado por request id.
func Logging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(sw, r)
		log.Printf("request_id=%s method=%s path=%s status=%d duration_ms=%d",
			requestIDFromContext(r.Context()), r.Method, r.URL.Path, sw.status, time.Since(start).Milliseconds())
	})
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

// SecurityHeaders aplica cabeceras mínimas de protección (XSS/clickjacking/
// sniffing), complementarias a CSRF (SameSite en cookies) y a la política
// exigida de TLS/cifrado en el borde (terminado por el proxy en producción).
func SecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		next.ServeHTTP(w, r)
	})
}

// CORS habilita el origen del frontend configurado, con credenciales
// (cookie de sesión) permitidas.
// CabecerasPermitidas son las que el navegador puede enviar desde otro
// origen.
//
// Se arma a partir de las constantes reales y no de una cadena suelta: la
// lista se había quedado sin X-CSRF-Token, así que el preflight rechazaba
// todo inicio de sesión desde el navegador aunque la API estuviera correcta.
var CabecerasPermitidas = []string{
	"Content-Type",
	"Authorization",
	CabeceraIdempotencia,
	"If-Match",
	CSRFHeaderName,
}

// CabecerasExpuestas son las que el JavaScript de la página puede leer de la
// respuesta. Sin declararlas, el navegador las oculta aunque viajen.
var CabecerasExpuestas = []string{
	"ETag",
	"X-Request-Id",
	CabeceraIdempotenciaRepetida,
}

func CORS(allowedOrigin string) func(http.Handler) http.Handler {
	permitidas := strings.Join(CabecerasPermitidas, ",")
	expuestas := strings.Join(CabecerasExpuestas, ",")

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			h := w.Header()
			h.Set("Access-Control-Allow-Origin", allowedOrigin)
			h.Set("Access-Control-Allow-Credentials", "true")
			h.Set("Access-Control-Allow-Methods", "GET,POST,PUT,PATCH,DELETE,OPTIONS")
			h.Set("Access-Control-Allow-Headers", permitidas)
			h.Set("Access-Control-Expose-Headers", expuestas)
			// El navegador puede reutilizar el preflight en lugar de repetirlo
			// antes de cada petición que cambia estado.
			h.Set("Access-Control-Max-Age", "600")
			// La respuesta depende del origen: sin esto, una caché intermedia
			// podría servirle a un origen la cabecera calculada para otro.
			h.Add("Vary", "Origin")
			if r.Method == http.MethodOptions {
				h.Add("Vary", "Access-Control-Request-Headers")
				w.WriteHeader(http.StatusNoContent)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// RateLimit aplica un límite de tasa fijo por ventana (por IP + ruta) usando
// Redis, protegiendo endpoints sensibles (login, registro, reset) de fuerza
// bruta y abuso.
func RateLimit(rdb *redis.Client, keyPrefix string, limit int, window time.Duration) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip := clientIP(r)
			key := "ratelimit:" + keyPrefix + ":" + ip
			ctx := r.Context()

			count, err := rdb.Incr(ctx, key).Result()
			if err != nil {
				// Redis no disponible: no se bloquea la solicitud (fail-open),
				// pero se registra para observabilidad.
				log.Printf("rate limit: redis no disponible: %v", err)
				next.ServeHTTP(w, r)
				return
			}
			if count == 1 {
				rdb.Expire(ctx, key, window)
			}
			if int(count) > limit {
				writeJSON(w, http.StatusTooManyRequests, errorEnvelope{Error: errorBody{
					Code: "rate_limited", Message: "demasiadas solicitudes, intenta más tarde",
				}})
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// clientIP devuelve la dirección del cliente, sin el puerto.
//
// RemoteAddr viene como "host:puerto", y el puerto es efímero: guardarlo en la
// bitácora y en las sesiones ensucia el dato y además impide agrupar por
// dirección, que es justo para lo que sirve.
// clientIP devuelve la dirección a la que atribuir la petición.
//
// La lista de proxies de confianza se instala al construir el router. Ver
// ip_del_cliente.go: X-Forwarded-For solo se cree cuando llega de un proxy
// declarado, porque de esta dirección dependen el límite de tasa y la
// bitácora.
func clientIP(r *http.Request) string {
	return ipDelCliente(r, proxiesConfiables)
}

// proxiesConfiables lo fija NewRouter al arrancar. Es un valor de paquete
// porque clientIP se llama desde middlewares que no reciben las dependencias.
var proxiesConfiables ProxiesDeConfianza

// RequireAuth exige una sesión activa y expone el usuario en el contexto.
func RequireAuth(authSvc *auth.Service) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token := extractToken(r)
			if token == "" {
				writeError(w, ErrUnauthenticated)
				return
			}
			u, sess, err := authSvc.Authenticate(r.Context(), token)
			if err != nil {
				writeError(w, ErrUnauthenticated)
				return
			}
			ctx := withSession(withUser(r.Context(), u), sess)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// RequireRole exige que el usuario autenticado tenga uno de los roles
// indicados; debe usarse después de RequireAuth.
func RequireRole(roles ...user.Role) func(http.Handler) http.Handler {
	allowed := make(map[user.Role]bool, len(roles))
	for _, r := range roles {
		allowed[r] = true
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			u, ok := UserFromContext(r.Context())
			if !ok {
				writeError(w, ErrUnauthenticated)
				return
			}
			if !allowed[u.Role] {
				writeError(w, ErrForbidden)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// Chain compone middlewares en orden de ejecución (el primero se ejecuta
// primero).
func Chain(h http.Handler, mws ...func(http.Handler) http.Handler) http.Handler {
	for i := len(mws) - 1; i >= 0; i-- {
		h = mws[i](h)
	}
	return h
}
