package httpserver

import (
	"context"
	"net/http"

	"github.com/google/uuid"

	"github.com/DES-SOLUCIONES-CLOUD/proyecto-1/backend/internal/domain/quiz"
	"github.com/DES-SOLUCIONES-CLOUD/proyecto-1/backend/internal/domain/user"
	"github.com/DES-SOLUCIONES-CLOUD/proyecto-1/backend/internal/platform/postgres"
)

// Metricas es lo que el panel administrativo necesita.
type Metricas interface {
	Resumen(ctx context.Context) (*postgres.Metricas, error)
	IntentosDeQuiz(ctx context.Context, quizID uuid.UUID) ([]quiz.IntentoCalificado, int, error)
}

func (h *handlers) registerMetricas(mux *http.ServeMux) {
	admin := RequireRole(user.RoleAdmin)
	teacherOrAdmin := RequireRole(user.RoleTeacher, user.RoleAdmin)

	mux.Handle("GET /api/v1/admin/metrics",
		h.auth()(admin(http.HandlerFunc(h.metricasDeLaPlataforma))))
	// Los resultados de una evaluación los ve su profesor, no solo la
	// administración: es quien puede corregir una pregunta mal planteada.
	mux.Handle("GET /api/v1/courses/versions/{versionId}/resources/{resourceId}/results",
		h.auth()(teacherOrAdmin(http.HandlerFunc(h.resultadosDeQuiz))))
}

func (h *handlers) metricasDeLaPlataforma(w http.ResponseWriter, r *http.Request) {
	if h.deps.Metricas == nil {
		writeError(w, ErrBadRequest)
		return
	}
	m, err := h.deps.Metricas.Resumen(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, m)
}

// resultadosDeQuiz agrega los intentos cerrados de una evaluación.
//
// La respuesta lleva la distribución por opción, incluida cuál era la
// correcta. No es una filtración: quien la ve es el autor del quiz o la
// administración, que ya la conocen, y sin ella el informe no serviría para lo
// que existe —ver qué distractor se está llevando a media clase—.
func (h *handlers) resultadosDeQuiz(w http.ResponseWriter, r *http.Request) {
	versionID, err1 := uuid.Parse(r.PathValue("versionId"))
	resourceID, err2 := uuid.Parse(r.PathValue("resourceId"))
	if err1 != nil || err2 != nil {
		writeError(w, ErrBadRequest)
		return
	}
	if h.deps.Metricas == nil || h.deps.Quizzes == nil {
		writeError(w, ErrBadRequest)
		return
	}
	actor, _ := UserFromContext(r.Context())

	// La autorización va primero y por la vía de siempre: el servicio de
	// quizzes comprueba que la versión es de quien pregunta.
	definicion, err := h.deps.Quizzes.DefinicionParaInforme(r.Context(), actor, versionID, resourceID)
	if err != nil {
		writeError(w, err)
		return
	}

	intentos, estudiantes, err := h.deps.Metricas.IntentosDeQuiz(r.Context(), definicion.Quiz.ID)
	if err != nil {
		writeError(w, err)
		return
	}

	resumen := quiz.Agregar(intentos, estudiantes, definicion.Textos)
	writeJSON(w, http.StatusOK, map[string]any{
		"quiz_id":    definicion.Quiz.ID,
		"title":      definicion.Quiz.Title,
		"resultados": resumen,
	})
}
