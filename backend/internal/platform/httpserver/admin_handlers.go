package httpserver

import (
	"net/http"
	"strconv"

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
