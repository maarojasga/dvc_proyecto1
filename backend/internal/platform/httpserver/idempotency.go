package httpserver

import (
	"bytes"
	"errors"
	"net/http"

	"github.com/redis/go-redis/v9"

	"github.com/DES-SOLUCIONES-CLOUD/proyecto-1/backend/internal/platform/redisclient"
)

// maxIdempotencyKeyLen acota la cabecera para que no se use como vía de
// almacenamiento arbitrario en Redis.
const maxIdempotencyKeyLen = 200

// capturingWriter conserva la respuesta para poder repetirla ante un
// reintento con la misma clave.
type capturingWriter struct {
	http.ResponseWriter
	status int
	body   bytes.Buffer
}

func (w *capturingWriter) WriteHeader(status int) {
	if w.status == 0 {
		w.status = status
		w.ResponseWriter.WriteHeader(status)
	}
}

func (w *capturingWriter) Write(p []byte) (int, error) {
	if w.status == 0 {
		w.WriteHeader(http.StatusOK)
	}
	w.body.Write(p)
	return w.ResponseWriter.Write(p)
}

// Idempotency hace repetible una operación marcada con Idempotency-Key.
//
// La primera petición se ejecuta y su respuesta queda guardada; las
// repeticiones con la misma clave devuelven esa respuesta sin volver a
// ejecutar nada. Es la garantía que el enunciado exige para el envío de
// quizzes y la emisión de insignias, resuelta aquí una sola vez para toda la
// API.
func Idempotency(rdb *redis.Client) func(http.Handler) http.Handler {
	store := redisclient.NuevoAlmacenIdempotencia(rdb)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			key := r.Header.Get("Idempotency-Key")
			if key == "" || safeMethod(r.Method) {
				next.ServeHTTP(w, r)
				return
			}
			if len(key) > maxIdempotencyKeyLen {
				writeError(w, ErrBadRequest)
				return
			}
			// La clave se combina con método y ruta para que la misma cadena
			// no colisione entre operaciones distintas.
			scope := r.Method + " " + r.URL.Path + " " + key

			previous, err := store.Reservar(r.Context(), scope)
			switch {
			case errors.Is(err, redisclient.ErrEnCurso):
				writeError(w, ErrIdempotencyInFlight)
				return
			case err != nil:
				// Sin idempotencia disponible se sigue adelante: es preferible
				// a rechazar la operación por un fallo de la caché.
				next.ServeHTTP(w, r)
				return
			case previous != nil:
				w.Header().Set("Idempotency-Replayed", "true")
				if previous.Tipo != "" {
					w.Header().Set("Content-Type", previous.Tipo)
				}
				w.WriteHeader(previous.Estado)
				_, _ = w.Write(previous.Cuerpo)
				return
			}

			cw := &capturingWriter{ResponseWriter: w}
			next.ServeHTTP(cw, r)
			if cw.status == 0 {
				cw.status = http.StatusOK
			}
			// Un 5xx se libera para que el cliente pueda reintentar con la
			// misma clave; solo se recuerdan las respuestas definitivas.
			if cw.status >= 500 {
				store.Liberar(r.Context(), scope)
				return
			}
			if err := store.Guardar(r.Context(), scope, redisclient.RespuestaGuardada{
				Estado: cw.status,
				Cuerpo: cw.body.Bytes(),
				Tipo:   cw.Header().Get("Content-Type"),
			}); err != nil {
				store.Liberar(r.Context(), scope)
			}
		})
	}
}
