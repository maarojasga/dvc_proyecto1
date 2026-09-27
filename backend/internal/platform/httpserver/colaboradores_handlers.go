package httpserver

import (
	"context"
	"net/http"

	"github.com/google/uuid"

	"github.com/DES-SOLUCIONES-CLOUD/proyecto-1/backend/internal/domain/user"
	"github.com/DES-SOLUCIONES-CLOUD/proyecto-1/backend/internal/platform/postgres"
)

// Colaboradores es la coautoría de un curso vista desde la API.
type Colaboradores interface {
	Listar(ctx context.Context, courseID uuid.UUID) ([]postgres.Colaborador, error)
	Agregar(ctx context.Context, courseID, userID, agregadoPor uuid.UUID) error
	Quitar(ctx context.Context, courseID, userID uuid.UUID) error
}

func (h *handlers) registerColaboradores(mux *http.ServeMux) {
	teacherOrAdmin := RequireRole(user.RoleTeacher, user.RoleAdmin)

	// Va bajo /collaborators y no bajo /courses/{courseId}/collaborators, que
	// sería lo natural: ese prefijo ya tiene registrado
	// /courses/versions/{versionId}, y net/http.ServeMux rechaza en el
	// arranque dos patrones donde un comodín compite con un literal en la
	// misma posición sin que ninguno sea más específico. Es el mismo motivo
	// por el que el progreso cuelga de /enrollments/{courseId}/progress.
	mux.Handle("GET /api/v1/collaborators/{courseId}",
		h.auth()(teacherOrAdmin(http.HandlerFunc(h.listarColaboradores))))
	mux.Handle("POST /api/v1/collaborators/{courseId}",
		h.auth()(teacherOrAdmin(http.HandlerFunc(h.agregarColaborador))))
	mux.Handle("DELETE /api/v1/collaborators/{courseId}/{userId}",
		h.auth()(teacherOrAdmin(http.HandlerFunc(h.quitarColaborador))))
}

// cursoDelDueno resuelve el curso exigiendo que quien pide sea su dueño.
//
// Repartir el acceso no se delega: un colaborador edita el curso, pero no
// puede añadir ni quitar a otros. Sin esta distinción, dar acceso a alguien
// equivaldría a cederle el curso.
func (h *handlers) cursoDelDueno(r *http.Request) (uuid.UUID, error) {
	courseID, err := uuid.Parse(r.PathValue("courseId"))
	if err != nil {
		return uuid.Nil, ErrBadRequest
	}
	actor, _ := UserFromContext(r.Context())
	c, err := h.deps.Courses.GetCourse(r.Context(), actor, courseID)
	if err != nil {
		return uuid.Nil, err
	}
	if !h.deps.Courses.EsDueno(actor, c) {
		return uuid.Nil, ErrForbidden
	}
	return courseID, nil
}

func (h *handlers) listarColaboradores(w http.ResponseWriter, r *http.Request) {
	courseID, err := h.cursoDelDueno(r)
	if err != nil {
		writeError(w, err)
		return
	}
	items, err := h.deps.Colaboradores.Listar(r.Context(), courseID)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

type agregarColaboradorRequest struct {
	Email string `json:"email"`
}

// agregarColaborador da acceso de edición por correo.
//
// Se identifica por correo y no por identificador porque es lo que el dueño
// tiene a mano. La respuesta no distingue "no existe" de "no es profesor": en
// los dos casos el acceso no se puede conceder, y separarlos convertiría este
// endpoint en una forma de averiguar qué correos están registrados.
func (h *handlers) agregarColaborador(w http.ResponseWriter, r *http.Request) {
	courseID, err := h.cursoDelDueno(r)
	if err != nil {
		writeError(w, err)
		return
	}
	var req agregarColaboradorRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	actor, _ := UserFromContext(r.Context())

	destinatario, err := h.deps.Admin.BuscarProfesorPorCorreo(r.Context(), req.Email)
	if err != nil {
		writeError(w, err)
		return
	}
	if destinatario.ID == actor.ID {
		// El dueño ya puede editar; añadirse a sí mismo solo ensucia la lista.
		writeJSON(w, http.StatusOK, map[string]string{"status": "sin_cambios"})
		return
	}

	if err := h.deps.Colaboradores.Agregar(r.Context(), courseID, destinatario.ID, actor.ID); err != nil {
		writeError(w, err)
		return
	}
	h.auditar(r, actor.ID, "course.collaborator_added", "course", courseID.String(), map[string]string{
		"colaborador": destinatario.Email,
	})
	writeJSON(w, http.StatusCreated, map[string]any{
		"user_id": destinatario.ID, "email": destinatario.Email, "role": "editor",
	})
}

func (h *handlers) quitarColaborador(w http.ResponseWriter, r *http.Request) {
	courseID, err := h.cursoDelDueno(r)
	if err != nil {
		writeError(w, err)
		return
	}
	userID, err := uuid.Parse(r.PathValue("userId"))
	if err != nil {
		writeError(w, ErrBadRequest)
		return
	}
	if err := h.deps.Colaboradores.Quitar(r.Context(), courseID, userID); err != nil {
		writeError(w, err)
		return
	}
	actor, _ := UserFromContext(r.Context())
	h.auditar(r, actor.ID, "course.collaborator_removed", "course", courseID.String(), map[string]string{
		"colaborador_id": userID.String(),
	})
	w.WriteHeader(http.StatusNoContent)
}
