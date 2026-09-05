package httpserver

import (
	"bytes"
	"crypto/subtle"
	"errors"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/DES-SOLUCIONES-CLOUD/proyecto-1/backend/internal/domain/user"
	"github.com/DES-SOLUCIONES-CLOUD/proyecto-1/backend/internal/platform/redisclient"
)

// middleware envuelve un handler.
type middleware func(http.Handler) http.Handler

// encadenar aplica los middlewares en el orden en que se listan: el primero
// es el mas externo.
func encadenar(h http.Handler, ms ...middleware) http.Handler {
	for i := len(ms) - 1; i >= 0; i-- {
		h = ms[i](h)
	}
	return h
}

// respuestaObservada captura el estado y el cuerpo para el log, la
// idempotencia y las metricas.
type respuestaObservada struct {
	http.ResponseWriter
	estado   int
	bytes    int
	capturar bool
	cuerpo   bytes.Buffer
}

func (w *respuestaObservada) WriteHeader(estado int) {
	if w.estado == 0 {
		w.estado = estado
		w.ResponseWriter.WriteHeader(estado)
	}
}

func (w *respuestaObservada) Write(p []byte) (int, error) {
	if w.estado == 0 {
		w.WriteHeader(http.StatusOK)
	}
	if w.capturar {
		w.cuerpo.Write(p)
	}
	n, err := w.ResponseWriter.Write(p)
	w.bytes += n
	return n, err
}

// conIdentificador asigna a cada peticion un identificador de correlacion y lo
// devuelve en la cabecera, para poder cruzar logs y trazas.
func conIdentificador(siguiente http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get("X-Request-Id")
		if id == "" {
			id = uuid.NewString()
		}
		w.Header().Set("X-Request-Id", id)
		siguiente.ServeHTTP(w, r.WithContext(conIDPeticion(r.Context(), id)))
	})
}

// conRegistro deja una linea estructurada por peticion.
func (s *Servidor) conRegistro(siguiente http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		inicio := time.Now()
		obs := &respuestaObservada{ResponseWriter: w}
		siguiente.ServeHTTP(obs, r)
		if obs.estado == 0 {
			obs.estado = http.StatusOK
		}
		s.log.InfoContext(r.Context(), "peticion",
			"metodo", r.Method,
			"ruta", r.URL.Path,
			"estado", obs.estado,
			"duracion_ms", time.Since(inicio).Milliseconds(),
			"peticion", IDPeticion(r.Context()),
			"ip", ipCliente(r),
		)
	})
}

// conRecuperacion evita que un panic tumbe el proceso y responde 500.
func (s *Servidor) conRecuperacion(siguiente http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if p := recover(); p != nil {
				s.log.ErrorContext(r.Context(), "panic atendiendo la peticion",
					"panic", p, "ruta", r.URL.Path, "peticion", IDPeticion(r.Context()))
				responderProblema(w, r, Problema{
					Type: TipoProblemaBase + "interno", Status: http.StatusInternalServerError,
					Title: "Error interno del servidor",
				})
			}
		}()
		siguiente.ServeHTTP(w, r)
	})
}

// conCabecerasSeguridad fija las cabeceras defensivas de toda respuesta.
func (s *Servidor) conCabecerasSeguridad(siguiente http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "no-referrer")
		// La API solo devuelve JSON: nada que ejecutar ni que incrustar.
		h.Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'")
		h.Set("Cache-Control", "no-store")
		if s.cookieSegura {
			h.Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		}
		siguiente.ServeHTTP(w, r)
	})
}

// conLimiteTasa acota las peticiones por IP en una ventana.
//
// Se aplica sobre todo a los endpoints de credenciales, donde frena el
// rociado de claves y el abuso del envio de correos.
func (s *Servidor) conLimiteTasa(nombre string, maximo int, ventana time.Duration) middleware {
	return func(siguiente http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if s.limitador == nil {
				siguiente.ServeHTTP(w, r)
				return
			}
			res := s.limitador.Permitir(r.Context(), nombre+":"+ipCliente(r), maximo, ventana)
			if !res.Permitido {
				w.Header().Set("Retry-After", strconv.Itoa(int(res.Reintento.Seconds())))
				responderProblema(w, r, Problema{
					Type: TipoProblemaBase + "limite-de-tasa", Status: http.StatusTooManyRequests,
					Title:  "Demasiadas peticiones",
					Detail: "Espera unos momentos antes de volver a intentarlo.",
				})
				return
			}
			siguiente.ServeHTTP(w, r)
		})
	}
}

// metodosSeguros son los que no cambian estado y por tanto no exigen CSRF.
func metodoSeguro(m string) bool {
	switch m {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return true
	}
	return false
}

// conCSRF aplica la defensa de doble envio de cookie.
//
// La cookie de sesion es SameSite=Lax, lo que ya bloquea la mayoria de los
// envios entre sitios. Esta comprobacion cubre el resto: toda peticion que
// modifique estado usando la cookie debe repetir en la cabecera el valor de
// la cookie CSRF, que un sitio atacante no puede leer.
func (s *Servidor) conCSRF(siguiente http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if metodoSeguro(r.Method) || tokenDeCookie(r, CookieSesion) == "" {
			siguiente.ServeHTTP(w, r)
			return
		}
		enCookie := tokenDeCookie(r, CookieCSRF)
		enCabecera := r.Header.Get(CabeceraCSRF)
		if enCookie == "" || subtle.ConstantTimeCompare([]byte(enCookie), []byte(enCabecera)) != 1 {
			responderProblema(w, r, Problema{
				Type: TipoProblemaBase + "csrf", Status: http.StatusForbidden,
				Title:  "Falta o no coincide el token anti-CSRF",
				Detail: "Vuelve a cargar la pagina e intentalo de nuevo.",
			})
			return
		}
		siguiente.ServeHTTP(w, r)
	})
}

// conAutenticacionOpcional resuelve la cookie de sesion si viene, sin exigirla.
func (s *Servidor) conAutenticacionOpcional(siguiente http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := tokenDeCookie(r, CookieSesion)
		if token == "" {
			siguiente.ServeHTTP(w, r)
			return
		}
		a, err := s.usuarios.Autenticar(r.Context(), token)
		switch {
		case errors.Is(err, user.ErrSesionInvalida):
			// La cookie ya no sirve: se limpia para que el navegador no la
			// siga enviando en cada peticion.
			s.borrarCookies(w)
			siguiente.ServeHTTP(w, r)
			return
		case err != nil:
			problemaDeDominio(w, r, s.log, err)
			return
		}
		siguiente.ServeHTTP(w, r.WithContext(conAutenticacion(r.Context(), a)))
	})
}

// requiereSesion corta con 401 si la peticion no viene autenticada.
func requiereSesion(siguiente http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, ok := autenticacionDe(r.Context()); !ok {
			responderNoAutenticado(w, r)
			return
		}
		siguiente.ServeHTTP(w, r)
	})
}

// conIdempotencia hace repetible una operacion marcada con Idempotency-Key.
//
// La primera peticion se ejecuta y su respuesta queda guardada; las repeticiones
// con la misma clave devuelven esa respuesta sin volver a ejecutar nada. Es la
// misma garantia que el enunciado exige para el envio de quizzes y la emision
// de insignias, resuelta aqui una sola vez para toda la API.
func (s *Servidor) conIdempotencia(siguiente http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		clave := r.Header.Get("Idempotency-Key")
		if clave == "" || s.idempotencia == nil || metodoSeguro(r.Method) {
			siguiente.ServeHTTP(w, r)
			return
		}
		if len(clave) > 200 {
			responderProblema(w, r, Problema{
				Type: TipoProblemaBase + "validacion", Status: http.StatusBadRequest,
				Title: "La cabecera Idempotency-Key es demasiado larga",
			})
			return
		}
		// La clave se combina con metodo y ruta para que la misma cadena no
		// colisione entre operaciones distintas.
		alcance := r.Method + " " + r.URL.Path + " " + clave

		previa, err := s.idempotencia.Reservar(r.Context(), alcance)
		switch {
		case errors.Is(err, redisclient.ErrEnCurso):
			responderProblema(w, r, Problema{
				Type: TipoProblemaBase + "peticion-en-curso", Status: http.StatusConflict,
				Title:  "Ya hay una peticion identica en curso",
				Detail: "Espera a que termine antes de reintentar.",
			})
			return
		case err != nil:
			// Sin idempotencia disponible se sigue adelante: es preferible a
			// rechazar la operacion por un fallo de la cache.
			s.log.WarnContext(r.Context(), "idempotencia no disponible", "error", err)
			siguiente.ServeHTTP(w, r)
			return
		case previa != nil:
			w.Header().Set("Idempotency-Replayed", "true")
			if previa.Tipo != "" {
				w.Header().Set("Content-Type", previa.Tipo)
			}
			w.WriteHeader(previa.Estado)
			_, _ = w.Write(previa.Cuerpo)
			return
		}

		obs := &respuestaObservada{ResponseWriter: w, capturar: true}
		siguiente.ServeHTTP(obs, r)
		if obs.estado == 0 {
			obs.estado = http.StatusOK
		}
		// Solo se recuerdan las respuestas definitivas. Un 5xx se libera para
		// que el cliente pueda reintentar con la misma clave.
		if obs.estado >= 500 {
			s.idempotencia.Liberar(r.Context(), alcance)
			return
		}
		if err := s.idempotencia.Guardar(r.Context(), alcance, redisclient.RespuestaGuardada{
			Estado: obs.estado,
			Cuerpo: obs.cuerpo.Bytes(),
			Tipo:   obs.Header().Get("Content-Type"),
		}); err != nil {
			s.log.WarnContext(r.Context(), "no se pudo guardar la idempotencia", "error", err)
		}
	})
}

// ipCliente devuelve la IP del cliente, respetando X-Forwarded-For cuando la
// API corre detras de un proxy.
func ipCliente(r *http.Request) string {
	if reenviada := r.Header.Get("X-Forwarded-For"); reenviada != "" {
		// El primer elemento es el cliente original.
		primera, _, _ := strings.Cut(reenviada, ",")
		return strings.TrimSpace(primera)
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
