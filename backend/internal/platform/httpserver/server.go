// Package httpserver adapta los casos de uso de internal/app a HTTP/JSON
// bajo el prefijo /api/v1, con middleware de autenticación, autorización,
// límites de tasa y manejo uniforme de errores.
package httpserver

import (
	"net/http"
	"strings"
	"time"

	"github.com/hibiken/asynq"
	"github.com/redis/go-redis/v9"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"

	"github.com/DES-SOLUCIONES-CLOUD/proyecto-1/backend/internal/app/admin"
	"github.com/DES-SOLUCIONES-CLOUD/proyecto-1/backend/internal/app/auth"
	"github.com/DES-SOLUCIONES-CLOUD/proyecto-1/backend/internal/app/courses"
	"github.com/DES-SOLUCIONES-CLOUD/proyecto-1/backend/internal/app/enrollments"
	"github.com/DES-SOLUCIONES-CLOUD/proyecto-1/backend/internal/app/progreso"
	"github.com/DES-SOLUCIONES-CLOUD/proyecto-1/backend/internal/app/quizzes"
	"github.com/DES-SOLUCIONES-CLOUD/proyecto-1/backend/internal/platform/antimalware"
	"github.com/DES-SOLUCIONES-CLOUD/proyecto-1/backend/internal/platform/postgres"
)

type Deps struct {
	Auth        *auth.Service
	Admin       *admin.Service
	Courses     *courses.Service
	Enrollments *enrollments.Service
	Quizzes     *quizzes.Service
	Progreso    *progreso.Service
	Media       *postgres.MediaRepo
	Storage     AlmacenDeCargas
	// Antimalware escanea lo que se sube antes de darlo por bueno. Es
	// obligatorio: NewRouter no acepta nil, porque un flujo de carga sin
	// escaneo que parece tenerlo es peor que no tenerlo.
	Antimalware antimalware.Escaner
	// Entrega resuelve las URL de lectura. En producción es el mismo cliente
	// de Storage; se declara aparte porque la reproducción solo necesita eso.
	Entrega      EntregaDeObjetos
	Redis        *redis.Client
	Queue        *asynq.Client
	Inspector    *asynq.Inspector
	CORSOrigin   string
	CookieSecure bool
	// ProxiesDeConfianza son las redes desde las que se acepta
	// X-Forwarded-For, en CIDR separados por comas. Detrás del proxy inverso
	// del despliegue hay que declararlo: sin esto todas las peticiones
	// parecerían venir del proxy y un solo abusador agotaría el límite de
	// tasa de todos. Vacío significa que no se cree a nadie, que es el lado
	// seguro por defecto.
	ProxiesDeConfianza string
}

func NewRouter(d Deps) http.Handler {
	proxiesConfiables = NuevosProxiesDeConfianza(d.ProxiesDeConfianza)

	mux := http.NewServeMux()

	h := &handlers{deps: d}
	h.registerHealth(mux)
	h.registerAuth(mux)
	h.registerAdmin(mux)
	h.registerCourses(mux)
	h.registerEnrollments(mux)
	h.registerQuizzes(mux)
	h.registerProgreso(mux)
	h.registerMedia(mux)
	h.registerOperacion(mux)

	manejador := Chain(mux,
		RequestID,
		Recover,
		Logging,
		SecurityHeaders,
		CORS(d.CORSOrigin),
		CSRF,
		Idempotency(d.Redis),
	)

	// La instrumentación va por fuera de todo: así el tramo cubre también lo
	// que hacen los middlewares (el límite de tasa, la idempotencia, el
	// rechazo por CSRF), que es justo donde conviene mirar cuando una petición
	// no llega al handler.
	//
	// El nombre del tramo sale del patrón de ruta y no de la URL concreta: con
	// la URL, cada identificador crearía un nombre distinto y las métricas
	// serían inservibles por cardinalidad.
	return otelhttp.NewHandler(manejador, "api",
		otelhttp.WithSpanNameFormatter(func(_ string, r *http.Request) string {
			if patron := patronDeRuta(r); patron != "" {
				return r.Method + " " + patron
			}
			return r.Method
		}),
		// El chequeo de salud lo llama el orquestador cada pocos segundos y no
		// dice nada de nadie: instrumentarlo solo añade ruido y coste.
		otelhttp.WithFilter(func(r *http.Request) bool {
			return r.URL.Path != "/api/v1/health"
		}),
	)
}

// patronDeRuta devuelve el patrón con el que se registró la ruta
// ("GET /api/v1/courses/{courseId}"), sin los valores concretos.
func patronDeRuta(r *http.Request) string {
	if r.Pattern == "" {
		return ""
	}
	// El patrón viene como "METODO /ruta"; el método ya se añade aparte.
	if i := strings.IndexByte(r.Pattern, ' '); i >= 0 {
		return r.Pattern[i+1:]
	}
	return r.Pattern
}

type handlers struct {
	deps Deps
}

func (h *handlers) auth() func(http.Handler) http.Handler {
	return RequireAuth(h.deps.Auth)
}

func loginRateLimit(rdb *redis.Client) func(http.Handler) http.Handler {
	return RateLimit(rdb, "auth", 10, time.Minute)
}
