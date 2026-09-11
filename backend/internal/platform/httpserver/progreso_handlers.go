package httpserver

import (
	"net/http"

	"github.com/google/uuid"

	"github.com/DES-SOLUCIONES-CLOUD/proyecto-1/backend/internal/domain/user"
)

func (h *handlers) registerProgreso(mux *http.ServeMux) {
	estudiante := RequireRole(user.RoleStudent)
	admin := RequireRole(user.RoleAdmin)

	mux.Handle("POST /api/v1/recursos/{resourceId}/progreso",
		h.auth()(estudiante(http.HandlerFunc(h.registrarProgreso))))
	mux.Handle("GET /api/v1/cursos/{courseId}/progreso",
		h.auth()(estudiante(http.HandlerFunc(h.verProgreso))))
	mux.Handle("GET /api/v1/insignias/mias",
		h.auth()(estudiante(http.HandlerFunc(h.misInsignias))))
	mux.Handle("POST /api/v1/insignias/{codigo}/revocar",
		h.auth()(admin(http.HandlerFunc(h.revocarInsignia))))

	// La verificacion es publica y sin sesion: ese es justamente el punto de
	// una insignia verificable.
	mux.Handle("GET /api/v1/insignias/{codigo}", http.HandlerFunc(h.verificarInsignia))
}

type eventoProgresoRequest struct {
	Type     string `json:"type"`
	Complete bool   `json:"complete"`
}

func (h *handlers) registrarProgreso(w http.ResponseWriter, r *http.Request) {
	resourceID, err := uuid.Parse(r.PathValue("resourceId"))
	if err != nil {
		writeError(w, ErrBadRequest)
		return
	}
	var req eventoProgresoRequest
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

func (h *handlers) verProgreso(w http.ResponseWriter, r *http.Request) {
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

func (h *handlers) misInsignias(w http.ResponseWriter, r *http.Request) {
	actor, _ := UserFromContext(r.Context())
	lista, err := h.deps.Progreso.MisInsignias(r.Context(), actor)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": lista})
}

func (h *handlers) verificarInsignia(w http.ResponseWriter, r *http.Request) {
	v, err := h.deps.Progreso.Verificar(r.Context(), r.PathValue("codigo"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, v)
}

type revocarInsigniaRequest struct {
	Reason string `json:"reason"`
}

func (h *handlers) revocarInsignia(w http.ResponseWriter, r *http.Request) {
	var req revocarInsigniaRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	actor, _ := UserFromContext(r.Context())
	if err := h.deps.Progreso.Revocar(r.Context(), actor, r.PathValue("codigo"), req.Reason); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "revoked"})
}
