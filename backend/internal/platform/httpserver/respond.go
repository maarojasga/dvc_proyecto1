package httpserver

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"

	"github.com/DES-SOLUCIONES-CLOUD/proyecto-1/backend/internal/app/auth"
	"github.com/DES-SOLUCIONES-CLOUD/proyecto-1/backend/internal/app/courses"
	"github.com/DES-SOLUCIONES-CLOUD/proyecto-1/backend/internal/app/enrollments"
	"github.com/DES-SOLUCIONES-CLOUD/proyecto-1/backend/internal/app/progreso"
	"github.com/DES-SOLUCIONES-CLOUD/proyecto-1/backend/internal/app/quizzes"
	"github.com/DES-SOLUCIONES-CLOUD/proyecto-1/backend/internal/domain/enrollment"
	"github.com/DES-SOLUCIONES-CLOUD/proyecto-1/backend/internal/domain/quiz"
	"github.com/DES-SOLUCIONES-CLOUD/proyecto-1/backend/internal/domain/user"
	"github.com/DES-SOLUCIONES-CLOUD/proyecto-1/backend/internal/platform/postgres"
)

// errorEnvelope es el formato uniforme de error exigido por la restricción
// técnica de "errores uniformes" de la API REST.
type errorEnvelope struct {
	Error errorBody `json:"error"`
}

type errorBody struct {
	Code    string   `json:"code"`
	Message string   `json:"message"`
	Details []string `json:"details,omitempty"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if v != nil {
		_ = json.NewEncoder(w).Encode(v)
	}
}

func writeError(w http.ResponseWriter, err error) {
	status, code, details := classifyError(err)
	if status >= 500 {
		log.Printf("error interno: %v", err)
	}
	writeJSON(w, status, errorEnvelope{Error: errorBody{Code: code, Message: err.Error(), Details: details}})
}

// classifyError traduce errores de dominio y de aplicación a códigos HTTP y
// de error estables, sin acoplar el dominio a net/http.
func classifyError(err error) (status int, code string, details []string) {
	var valErr *domainValidationError
	if errors.As(err, &valErr) {
		return http.StatusUnprocessableEntity, "validation_failed", valErr.Details
	}

	switch {
	case errors.Is(err, postgres.ErrNotFound):
		return http.StatusNotFound, "not_found", nil
	case errors.Is(err, auth.ErrInvalidCredentials):
		return http.StatusUnauthorized, "invalid_credentials", nil
	case errors.Is(err, auth.ErrEmailInUse):
		// Solo lo produce el alta de profesores por administración, donde el
		// administrador es de confianza. El registro público nunca lo
		// devuelve: allí revelaría qué correos existen.
		return http.StatusConflict, "email_in_use", nil
	case errors.Is(err, auth.ErrSessionNotFound):
		return http.StatusNotFound, "session_not_found", nil
	case errors.Is(err, auth.ErrInvalidOrExpiredToken):
		return http.StatusBadRequest, "invalid_or_expired_token", nil
	case errors.Is(err, user.ErrWeakPassword):
		return http.StatusUnprocessableEntity, "weak_password", nil
	case errors.Is(err, user.ErrAccountNotActive):
		return http.StatusForbidden, "account_not_active", nil
	case errors.Is(err, user.ErrLastAdminProtected):
		return http.StatusConflict, "last_admin_protected", nil
	case errors.Is(err, courses.ErrForbidden):
		return http.StatusForbidden, "forbidden", nil
	case errors.Is(err, courses.ErrVersionNotDraft):
		return http.StatusConflict, "version_not_draft", nil
	case errors.Is(err, enrollment.ErrAlreadyEnrolled):
		return http.StatusConflict, "already_enrolled", nil
	case errors.Is(err, enrollment.ErrNotEnrolled):
		return http.StatusNotFound, "not_enrolled", nil
	case errors.Is(err, enrollments.ErrMediaNoLista):
		return http.StatusConflict, "media_not_ready", nil
	case errors.Is(err, enrollments.ErrPosicionInvalida):
		return http.StatusUnprocessableEntity, "invalid_position", nil
	case errors.Is(err, enrollment.ErrCourseNotPublished):
		return http.StatusConflict, "course_not_published", nil
	case errors.Is(err, quiz.ErrMaxAttemptsReached):
		return http.StatusConflict, "max_attempts_reached", nil
	case errors.Is(err, quiz.ErrAttemptExpired):
		return http.StatusConflict, "attempt_expired", nil
	case errors.Is(err, quiz.ErrAttemptNotOpen):
		return http.StatusConflict, "attempt_not_open", nil
	case errors.Is(err, quiz.ErrAlreadySubmitted):
		return http.StatusConflict, "attempt_already_submitted", nil
	case errors.Is(err, quizzes.ErrRespuestaInvalida):
		return http.StatusUnprocessableEntity, "invalid_answer", nil
	case errors.Is(err, quizzes.ErrIntentoAjeno):
		// Se responde como inexistente y no como prohibido: confirmar que el
		// intento existe ya seria filtrar informacion de otro estudiante.
		return http.StatusNotFound, "not_found", nil
	case errors.Is(err, progreso.ErrTipoDeEventoInvalido):
		return http.StatusUnprocessableEntity, "invalid_event_type", nil
	case errors.Is(err, ErrUnauthenticated):
		return http.StatusUnauthorized, "unauthenticated", nil
	case errors.Is(err, ErrForbidden):
		return http.StatusForbidden, "forbidden", nil
	case errors.Is(err, ErrBadRequest):
		return http.StatusBadRequest, "bad_request", nil
	case errors.Is(err, ErrCSRF):
		return http.StatusForbidden, "csrf_token_mismatch", nil
	case errors.Is(err, ErrIdempotencyInFlight):
		return http.StatusConflict, "idempotency_in_flight", nil
	case errors.Is(err, ErrColaNoDisponible):
		return http.StatusServiceUnavailable, "queue_unavailable", nil
	default:
		return http.StatusInternalServerError, "internal_error", nil
	}
}

// domainValidationError adapta course.ValidationError (lista exhaustiva de
// motivos) al formato uniforme sin que este paquete importe el tipo
// concreto en cada punto de uso.
type domainValidationError struct {
	Details []string
}

func (e *domainValidationError) Error() string { return "validación fallida" }

func asValidationError(reasons []error) error {
	details := make([]string, 0, len(reasons))
	for _, r := range reasons {
		details = append(details, r.Error())
	}
	return &domainValidationError{Details: details}
}
