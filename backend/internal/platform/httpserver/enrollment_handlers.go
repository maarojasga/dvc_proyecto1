package httpserver

import (
	"net/http"

	"github.com/google/uuid"

	"github.com/DES-SOLUCIONES-CLOUD/proyecto-1/backend/internal/domain/user"
)

func (h *handlers) registerEnrollments(mux *http.ServeMux) {
	student := RequireRole(user.RoleStudent)

	mux.Handle("POST /api/v1/enrollments", h.auth()(student(http.HandlerFunc(h.enroll))))
	mux.Handle("GET /api/v1/enrollments/mine", h.auth()(student(http.HandlerFunc(h.listMyEnrollments))))
	mux.Handle("POST /api/v1/enrollments/{courseId}/withdraw", h.auth()(student(http.HandlerFunc(h.withdraw))))
}

type enrollRequest struct {
	CourseID string `json:"course_id"`
}

func (h *handlers) enroll(w http.ResponseWriter, r *http.Request) {
	var req enrollRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	courseID, err := uuid.Parse(req.CourseID)
	if err != nil {
		writeError(w, ErrBadRequest)
		return
	}
	actor, _ := UserFromContext(r.Context())
	e, err := h.deps.Enrollments.Enroll(r.Context(), actor, courseID)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, e)
}

func (h *handlers) listMyEnrollments(w http.ResponseWriter, r *http.Request) {
	actor, _ := UserFromContext(r.Context())
	list, err := h.deps.Enrollments.ListMine(r.Context(), actor)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": list})
}

func (h *handlers) withdraw(w http.ResponseWriter, r *http.Request) {
	courseID, err := uuid.Parse(r.PathValue("courseId"))
	if err != nil {
		writeError(w, ErrBadRequest)
		return
	}
	actor, _ := UserFromContext(r.Context())
	if err := h.deps.Enrollments.Withdraw(r.Context(), actor, courseID); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "withdrawn"})
}
