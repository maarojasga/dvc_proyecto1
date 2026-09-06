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
	"github.com/DES-SOLUCIONES-CLOUD/proyecto-1/backend/internal/platform/postgres"
	"github.com/DES-SOLUCIONES-CLOUD/proyecto-1/backend/internal/platform/storage"
)

type Deps struct {
	Auth         *auth.Service
	Admin        *admin.Service
	Courses      *courses.Service
	Enrollments  *enrollments.Service
	Media        *postgres.MediaRepo
	Storage      *storage.Client
	Redis        *redis.Client
	Queue        *asynq.Client
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
