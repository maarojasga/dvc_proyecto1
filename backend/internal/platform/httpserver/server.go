// Package httpserver adapta los casos de uso de internal/app a HTTP/JSON
// bajo el prefijo /api/v1, con middleware de autenticación, autorización,
// límites de tasa y manejo uniforme de errores.
package httpserver

import (
	"net/http"
	"time"

	"github.com/hibiken/asynq"
	"github.com/redis/go-redis/v9"

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
}

func NewRouter(d Deps) http.Handler {
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

	return Chain(mux,
		RequestID,
		Recover,
		Logging,
		SecurityHeaders,
		CORS(d.CORSOrigin),
		CSRF,
		Idempotency(d.Redis),
	)
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
