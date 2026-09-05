package httpserver

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/DES-SOLUCIONES-CLOUD/proyecto-1/backend/internal/domain/user"
	"github.com/DES-SOLUCIONES-CLOUD/proyecto-1/backend/internal/platform/redisclient"
)

// Prefijo de version de la API.
const Prefijo = "/api/v1"

// Cupos de los endpoints sensibles: ventana y maximo de peticiones por IP.
const (
	ventanaCupo = time.Minute

	cupoLogin       = 10
	cupoRegistro    = 5
	cupoCorreos     = 3
	cupoRecuperar   = 5
	cupoGeneralLect = 120
)

// Servidor arma el router de la API.
type Servidor struct {
	usuarios     *user.Servicio
	log          *slog.Logger
	limitador    *redisclient.LimitadorTasa
	idempotencia *redisclient.AlmacenIdempotencia
	cookieSegura bool
}

// Opciones son las dependencias del servidor. Limitador e Idempotencia son
// opcionales: sin Redis la API sigue funcionando con menos defensas.
type Opciones struct {
	Usuarios     *user.Servicio
	Log          *slog.Logger
	Limitador    *redisclient.LimitadorTasa
	Idempotencia *redisclient.AlmacenIdempotencia
	CookieSegura bool
}

// Nuevo construye el servidor.
func Nuevo(o Opciones) *Servidor {
	log := o.Log
	if log == nil {
		log = slog.Default()
	}
	return &Servidor{
		usuarios: o.Usuarios, log: log,
		limitador: o.Limitador, idempotencia: o.Idempotencia,
		cookieSegura: o.CookieSegura,
	}
}

// Handler devuelve el arbol de rutas con los middlewares globales aplicados.
func (s *Servidor) Handler() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET "+Prefijo+"/health", s.salud)

	// Identidad. Los endpoints publicos llevan cupo por IP; los que operan
	// sobre la sesion exigen estar autenticado.
	mux.Handle("POST "+Prefijo+"/auth/registro",
		encadenar(http.HandlerFunc(s.registro),
			s.conLimiteTasa("registro", cupoRegistro, ventanaCupo),
			s.conIdempotencia))

	mux.Handle("POST "+Prefijo+"/auth/verificar-correo",
		encadenar(http.HandlerFunc(s.verificarCorreo),
			s.conLimiteTasa("verificar", cupoRegistro, ventanaCupo)))

	mux.Handle("POST "+Prefijo+"/auth/reenviar-verificacion",
		encadenar(http.HandlerFunc(s.reenviarVerificacion),
			s.conLimiteTasa("reenviar", cupoCorreos, ventanaCupo)))

	mux.Handle("POST "+Prefijo+"/auth/login",
		encadenar(http.HandlerFunc(s.login),
			s.conLimiteTasa("login", cupoLogin, ventanaCupo)))

	mux.Handle("POST "+Prefijo+"/auth/recuperar-clave",
		encadenar(http.HandlerFunc(s.solicitarRecuperacion),
			s.conLimiteTasa("recuperar", cupoRecuperar, ventanaCupo)))

	mux.Handle("POST "+Prefijo+"/auth/recuperar-clave/confirmar",
		encadenar(http.HandlerFunc(s.confirmarRecuperacion),
			s.conLimiteTasa("recuperar-confirmar", cupoRecuperar, ventanaCupo)))

	mux.Handle("POST "+Prefijo+"/auth/logout",
		encadenar(http.HandlerFunc(s.logout), requiereSesion))

	mux.Handle("GET "+Prefijo+"/auth/sesion",
		encadenar(http.HandlerFunc(s.sesionActual), requiereSesion))

	mux.Handle("GET "+Prefijo+"/auth/sesiones",
		encadenar(http.HandlerFunc(s.listarSesiones), requiereSesion))

	mux.Handle("DELETE "+Prefijo+"/auth/sesiones/{sesionID}",
		encadenar(http.HandlerFunc(s.revocarSesion), requiereSesion))

	mux.Handle("DELETE "+Prefijo+"/auth/sesiones",
		encadenar(http.HandlerFunc(s.revocarOtrasSesiones), requiereSesion))

	mux.HandleFunc("/", s.rutaNoEncontrada)

	return encadenar(mux,
		conIdentificador,
		s.conRecuperacion,
		s.conRegistro,
		s.conCabecerasSeguridad,
		s.conAutenticacionOpcional,
		s.conCSRF,
	)
}

func (s *Servidor) salud(w http.ResponseWriter, r *http.Request) {
	responderJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Servidor) rutaNoEncontrada(w http.ResponseWriter, r *http.Request) {
	responderProblema(w, r, Problema{
		Type: TipoProblemaBase + "no-encontrado", Status: http.StatusNotFound,
		Title: "Recurso no encontrado",
	})
}
