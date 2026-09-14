package httpserver

import (
	"net/http"

	"github.com/google/uuid"

	"github.com/DES-SOLUCIONES-CLOUD/proyecto-1/backend/internal/domain/user"
)

func (h *handlers) registerProgress(mux *http.ServeMux) {
	student := RequireRole(user.RoleStudent)
	admin := RequireRole(user.RoleAdmin)

	mux.Handle("POST /api/v1/resources/{resourceId}/progress",
		h.auth()(student(http.HandlerFunc(h.recordProgressEvent))))
	// Va bajo /enrollments y no bajo /courses/{courseId}/...: ese prefijo ya
	// tiene /courses/versions/{versionId} registrado, y un comodin compitiendo
	// con un literal en la misma posicion hace que net/http.ServeMux rechace
	// el patron como ambiguo en tiempo de arranque.
	mux.Handle("GET /api/v1/enrollments/{courseId}/progress",
		h.auth()(student(http.HandlerFunc(h.getCourseProgress))))
	mux.Handle("GET /api/v1/badges/mine",
		h.auth()(student(http.HandlerFunc(h.listMyBadges))))
	mux.Handle("POST /api/v1/badges/{code}/revoke",
		h.auth()(admin(http.HandlerFunc(h.revokeBadge))))

	// La verificacion es publica y sin sesion: ese es justamente el punto de
	// una insignia verificable.
	mux.Handle("GET /api/v1/badges/{code}", http.HandlerFunc(h.verifyBadge))
}

type progressEventRequest struct {
	Type     string `json:"type"`
	Complete bool   `json:"complete"`
}

func (h *handlers) recordProgressEvent(w http.ResponseWriter, r *http.Request) {
	resourceID, err := uuid.Parse(r.PathValue("resourceId"))
	if err != nil {
		writeError(w, ErrBadRequest)
		return
	}
	var req progressEventRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	actor, _ := UserFromContext(r.Context())
	res, err := h.deps.Progreso.RegistrarEvento(r.Context(), actor, resourceID, req.Type, req.Complete)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (h *handlers) getCourseProgress(w http.ResponseWriter, r *http.Request) {
	courseID, err := uuid.Parse(r.PathValue("courseId"))
	if err != nil {
		writeError(w, ErrBadRequest)
		return
	}
	actor, _ := UserFromContext(r.Context())
	res, err := h.deps.Progreso.Resumen(r.Context(), actor, courseID)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (h *handlers) listMyBadges(w http.ResponseWriter, r *http.Request) {
	actor, _ := UserFromContext(r.Context())
	list, err := h.deps.Progreso.MisInsignias(r.Context(), actor)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": list})
}

func (h *handlers) verifyBadge(w http.ResponseWriter, r *http.Request) {
	v, err := h.deps.Progreso.Verificar(r.Context(), r.PathValue("code"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, v)
}

type revokeBadgeRequest struct {
	Reason string `json:"reason"`
}

func (h *handlers) revokeBadge(w http.ResponseWriter, r *http.Request) {
	var req revokeBadgeRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	actor, _ := UserFromContext(r.Context())
	if err := h.deps.Progreso.Revocar(r.Context(), actor, r.PathValue("code"), req.Reason); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "revoked"})
}
