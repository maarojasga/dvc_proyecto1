// Package httpserver adapta los casos de uso de internal/app a HTTP/JSON
// bajo el prefijo /api/v1, con middleware de autenticación, autorización,
// límites de tasa y manejo uniforme de errores.
package httpserver

import (
	"context"
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
	"github.com/DES-SOLUCIONES-CLOUD/proyecto-1/backend/internal/platform/queue"
)

type Deps struct {
	Auth        *auth.Service
	Admin       *admin.Service
	Courses     *courses.Service
	Enrollments *enrollments.Service
	Quizzes     *quizzes.Service
	Progreso    *progreso.Service
	Media       *postgres.MediaRepo
	// Storage conduce las cargas de autoría. Es una interfaz estrecha para
	// que los manejadores no dependan del SDK del proveedor cloud.
	Storage AlmacenDeObjetos
	// Antimalware escanea cada objeto antes de aceptarlo. Vacío significa el
	// escáner integrado.
	Antimalware antimalware.Escaner
	// Iframes administra la lista blanca de destinos incrustables.
	Iframes ListaBlancaDeIframes
	// Auditor deja constancia en la bitácora inmutable de las acciones que no
	// pasan por un servicio de aplicación.
	Auditor Auditor
	// Entrega resuelve las URL de lectura. En producción es el mismo cliente
	// de Storage; se declara aparte porque la reproducción solo necesita eso.
	Entrega EntregaDeObjetos
	Redis   *redis.Client
	// Queue publica los trabajos asíncronos. Es una interfaz para que los
	// manejadores no dependan del SDK de la cola y una prueba pueda comprobar
	// qué se encoló sin Redis de por medio.
	Queue        queue.Encolador
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
	h.registerProgress(mux)
	h.registerMedia(mux)
	h.registerIframes(mux)
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

// Auditor es la bitácora inmutable vista desde la capa HTTP.
type Auditor interface {
	InsertAudit(ctx context.Context, e postgres.AuditEntry) error
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
