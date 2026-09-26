package httpserver

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"

	"github.com/DES-SOLUCIONES-CLOUD/proyecto-1/backend/internal/domain/user"
	"github.com/DES-SOLUCIONES-CLOUD/proyecto-1/backend/internal/platform/postgres"
)

func (h *handlers) registerAdmin(mux *http.ServeMux) {
	admin := RequireRole(user.RoleAdmin)

	mux.Handle("GET /api/v1/admin/users", h.auth()(admin(http.HandlerFunc(h.listUsers))))
	mux.Handle("PATCH /api/v1/admin/users/{id}/role", h.auth()(admin(http.HandlerFunc(h.updateUserRole))))
	mux.Handle("PATCH /api/v1/admin/users/{id}/status", h.auth()(admin(http.HandlerFunc(h.updateUserStatus))))
	mux.Handle("POST /api/v1/admin/teachers", h.auth()(admin(http.HandlerFunc(h.createTeacher))))
	mux.Handle("GET /api/v1/admin/audit", h.auth()(admin(http.HandlerFunc(h.listAudit))))
	mux.Handle("GET /api/v1/admin/users/{id}/sessions", h.auth()(admin(http.HandlerFunc(h.listUserSessions))))
	mux.Handle("DELETE /api/v1/admin/users/{id}/sessions", h.auth()(admin(http.HandlerFunc(h.revokeUserSessions))))
}

type auditResponse struct {
	ID         string          `json:"id"`
	ActorID    string          `json:"actor_id,omitempty"`
	ActorEmail string          `json:"actor_email,omitempty"`
	Action     string          `json:"action"`
	EntityType string          `json:"entity_type"`
	EntityID   string          `json:"entity_id,omitempty"`
	Metadata   json.RawMessage `json:"metadata"`
	IPAddress  string          `json:"ip_address,omitempty"`
	CreatedAt  string          `json:"created_at"`
}

// listAudit consulta la bitácora inmutable, la más reciente primero.
func (h *handlers) listAudit(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit, _ := strconv.Atoi(q.Get("limit"))
	offset, _ := strconv.Atoi(q.Get("offset"))

	filtro := postgres.ListAuditFilter{Action: q.Get("action"), Limit: limit, Offset: offset}
	if actor := q.Get("actor_id"); actor != "" {
		id, err := uuid.Parse(actor)
		if err != nil {
			writeError(w, ErrBadRequest)
			return
		}
		filtro.ActorID = &id
	}

	registros, err := h.deps.Admin.ListAudit(r.Context(), filtro)
	if err != nil {
		writeError(w, err)
		return
	}
	items := make([]auditResponse, 0, len(registros))
	for _, a := range registros {
		item := auditResponse{
			ID: a.ID.String(), ActorEmail: a.ActorEmail, Action: a.Action,
			EntityType: a.EntityType, EntityID: a.EntityID,
			Metadata: json.RawMessage(a.Metadata), IPAddress: a.IPAddress,
			CreatedAt: a.CreatedAt.Format(time.RFC3339),
		}
		if a.ActorID != nil {
			item.ActorID = a.ActorID.String()
		}
		items = append(items, item)
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

// listUserSessions muestra las sesiones activas de una cuenta.
func (h *handlers) listUserSessions(w http.ResponseWriter, r *http.Request) {
	targetID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeError(w, ErrBadRequest)
		return
	}
	sessions, err := h.deps.Admin.ListUserSessions(r.Context(), targetID)
	if err != nil {
		writeError(w, err)
		return
	}
	items := make([]sessionResponse, 0, len(sessions))
	for _, s := range sessions {
		items = append(items, sessionResponse{
			ID:        s.ID.String(),
			CreatedAt: s.CreatedAt.Format(time.RFC3339),
			ExpiresAt: s.ExpiresAt.Format(time.RFC3339),
			IPAddress: s.IPAddress,
			UserAgent: s.UserAgent,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

// revokeUserSessions expulsa a una cuenta de todos sus dispositivos sin
// cambiar su estado.
func (h *handlers) revokeUserSessions(w http.ResponseWriter, r *http.Request) {
	targetID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeError(w, ErrBadRequest)
		return
	}
	actor, _ := UserFromContext(r.Context())
	if err := h.deps.Admin.RevokeUserSessions(r.Context(), actor, targetID); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *handlers) listUsers(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit, _ := strconv.Atoi(q.Get("limit"))
	offset, _ := strconv.Atoi(q.Get("offset"))

	users, err := h.deps.Admin.ListUsers(r.Context(), postgres.ListUsersFilter{
		Role: q.Get("role"), Status: q.Get("status"), Search: q.Get("q"), Limit: limit, Offset: offset,
	})
	if err != nil {
		writeError(w, err)
		return
	}
	out := make([]userResponse, 0, len(users))
	for _, u := range users {
		out = append(out, userResponse{ID: u.ID.String(), Email: u.Email, FullName: u.FullName, Role: string(u.Role), Status: string(u.Status)})
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": out})
}

type updateRoleRequest struct {
	Role string `json:"role"`
}

func (h *handlers) updateUserRole(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeError(w, ErrBadRequest)
		return
	}
	var req updateRoleRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	actor, _ := UserFromContext(r.Context())
	if err := h.deps.Admin.UpdateRole(r.Context(), actor, id, user.Role(req.Role)); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

type updateStatusRequest struct {
	Status string `json:"status"`
}

func (h *handlers) updateUserStatus(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeError(w, ErrBadRequest)
		return
	}
	var req updateStatusRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	actor, _ := UserFromContext(r.Context())
	if err := h.deps.Admin.UpdateStatus(r.Context(), actor, id, user.Status(req.Status)); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

type createTeacherRequest struct {
	Email    string `json:"email"`
	FullName string `json:"full_name"`
}

func (h *handlers) createTeacher(w http.ResponseWriter, r *http.Request) {
	var req createTeacherRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	u, err := h.deps.Auth.CreateTeacher(r.Context(), req.Email, req.FullName)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, userResponse{ID: u.ID.String(), Email: u.Email, FullName: u.FullName, Role: string(u.Role), Status: string(u.Status)})
}
