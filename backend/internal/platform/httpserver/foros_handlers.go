package httpserver

import (
	"context"
	"net/http"

	"github.com/google/uuid"

	"github.com/DES-SOLUCIONES-CLOUD/proyecto-1/backend/internal/domain/user"
	"github.com/DES-SOLUCIONES-CLOUD/proyecto-1/backend/internal/platform/postgres"
)

// Foros es el foro asíncrono de un curso.
type Foros interface {
	CrearHilo(ctx context.Context, h *postgres.Hilo) error
	ListarHilos(ctx context.Context, courseID uuid.UUID, recurso *uuid.UUID) ([]postgres.Hilo, error)
	Hilo(ctx context.Context, id uuid.UUID) (*postgres.Hilo, error)
	Responder(ctx context.Context, p *postgres.Respuesta) error
	Respuestas(ctx context.Context, threadID uuid.UUID) ([]postgres.Respuesta, error)
	EliminarRespuesta(ctx context.Context, postID uuid.UUID) (*uuid.UUID, error)
	AutorDeRespuesta(ctx context.Context, postID uuid.UUID) (uuid.UUID, uuid.UUID, error)
	BloquearHilo(ctx context.Context, threadID uuid.UUID, bloqueado bool) error
}

func (h *handlers) registerForos(mux *http.ServeMux) {
	// Va bajo /forum y no bajo /courses/{courseId}/..., por el mismo conflicto
	// de patrones que llevó el progreso a /enrollments/{courseId}/progress.
	mux.Handle("GET /api/v1/forum/{courseId}", h.auth()(http.HandlerFunc(h.listarHilos)))
	mux.Handle("POST /api/v1/forum/{courseId}", h.auth()(http.HandlerFunc(h.crearHilo)))
	mux.Handle("GET /api/v1/forum/threads/{threadId}", h.auth()(http.HandlerFunc(h.verHilo)))
	mux.Handle("POST /api/v1/forum/threads/{threadId}/replies", h.auth()(http.HandlerFunc(h.responder)))
	mux.Handle("DELETE /api/v1/forum/posts/{postId}", h.auth()(http.HandlerFunc(h.eliminarRespuesta)))
	mux.Handle("POST /api/v1/forum/threads/{threadId}/lock",
		h.auth()(RequireRole(user.RoleTeacher, user.RoleAdmin)(http.HandlerFunc(h.bloquearHilo))))
}

// accesoAlForo autoriza a participar en el foro de un curso.
//
// El foro es del curso, así que se puede leer y escribir si se tiene acceso a
// su contenido: el estudiante inscrito, el profesor dueño o colaborador, y la
// administración. Un curso no es un tablón público.
func (h *handlers) accesoAlForo(r *http.Request, courseID uuid.UUID) error {
	actor, _ := UserFromContext(r.Context())

	// El profesor (dueño o colaborador) y la administración entran por la vía
	// de autoría; el estudiante, por su inscripción.
	if actor.Role == user.RoleAdmin || actor.Role == user.RoleTeacher {
		if _, err := h.deps.Courses.GetCourse(r.Context(), actor, courseID); err == nil {
			return nil
		}
	}
	if _, err := h.deps.Progreso.Resumen(r.Context(), actor, courseID); err != nil {
		return ErrForbidden
	}
	return nil
}

func (h *handlers) listarHilos(w http.ResponseWriter, r *http.Request) {
	courseID, err := uuid.Parse(r.PathValue("courseId"))
	if err != nil {
		writeError(w, ErrBadRequest)
		return
	}
	if err := h.accesoAlForo(r, courseID); err != nil {
		writeError(w, err)
		return
	}

	// Filtrar por recurso permite enseñar, junto a una lección, solo lo que se
	// discute sobre ella.
	var recurso *uuid.UUID
	if crudo := r.URL.Query().Get("resource_stable_id"); crudo != "" {
		id, err := uuid.Parse(crudo)
		if err != nil {
			writeError(w, ErrBadRequest)
			return
		}
		recurso = &id
	}

	hilos, err := h.deps.Foros.ListarHilos(r.Context(), courseID, recurso)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": hilos})
}

type crearHiloRequest struct {
	Titulo          string `json:"title"`
	CuerpoMD        string `json:"body_md"`
	RecursoStableID string `json:"resource_stable_id,omitempty"`
}

func (h *handlers) crearHilo(w http.ResponseWriter, r *http.Request) {
	courseID, err := uuid.Parse(r.PathValue("courseId"))
	if err != nil {
		writeError(w, ErrBadRequest)
		return
	}
	if err := h.accesoAlForo(r, courseID); err != nil {
		writeError(w, err)
		return
	}
	var req crearHiloRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.Titulo == "" || req.CuerpoMD == "" {
		writeError(w, ErrBadRequest)
		return
	}
	actor, _ := UserFromContext(r.Context())

	hilo := postgres.Hilo{
		CourseID: courseID, AutorID: &actor.ID,
		Titulo: req.Titulo, CuerpoMD: req.CuerpoMD,
	}
	if req.RecursoStableID != "" {
		id, err := uuid.Parse(req.RecursoStableID)
		if err != nil {
			writeError(w, ErrBadRequest)
			return
		}
		hilo.RecursoStableID = &id
	}
	if err := h.deps.Foros.CrearHilo(r.Context(), &hilo); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, hilo)
}

// hiloAutorizado resuelve un hilo comprobando el acceso a su curso.
func (h *handlers) hiloAutorizado(r *http.Request) (*postgres.Hilo, error) {
	threadID, err := uuid.Parse(r.PathValue("threadId"))
	if err != nil {
		return nil, ErrBadRequest
	}
	hilo, err := h.deps.Foros.Hilo(r.Context(), threadID)
	if err != nil {
		return nil, err
	}
	if err := h.accesoAlForo(r, hilo.CourseID); err != nil {
		return nil, err
	}
	return hilo, nil
}

func (h *handlers) verHilo(w http.ResponseWriter, r *http.Request) {
	hilo, err := h.hiloAutorizado(r)
	if err != nil {
		writeError(w, err)
		return
	}
	respuestas, err := h.deps.Foros.Respuestas(r.Context(), hilo.ID)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"thread": hilo, "replies": respuestas})
}

type responderRequest struct {
	CuerpoMD string `json:"body_md"`
}

func (h *handlers) responder(w http.ResponseWriter, r *http.Request) {
	hilo, err := h.hiloAutorizado(r)
	if err != nil {
		writeError(w, err)
		return
	}
	if hilo.Bloqueado {
		writeError(w, ErrHiloBloqueado)
		return
	}
	var req responderRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.CuerpoMD == "" {
		writeError(w, ErrBadRequest)
		return
	}
	actor, _ := UserFromContext(r.Context())

	p := postgres.Respuesta{ThreadID: hilo.ID, AutorID: &actor.ID, CuerpoMD: req.CuerpoMD}
	if err := h.deps.Foros.Responder(r.Context(), &p); err != nil {
		writeError(w, err)
		return
	}
	p.AutorNombre = actor.FullName
	writeJSON(w, http.StatusCreated, p)
}

// eliminarRespuesta la oculta. La puede borrar su autor o el profesor del
// curso; la administración, siempre.
func (h *handlers) eliminarRespuesta(w http.ResponseWriter, r *http.Request) {
	postID, err := uuid.Parse(r.PathValue("postId"))
	if err != nil {
		writeError(w, ErrBadRequest)
		return
	}
	autor, cursoID, err := h.deps.Foros.AutorDeRespuesta(r.Context(), postID)
	if err != nil {
		writeError(w, err)
		return
	}
	actor, _ := UserFromContext(r.Context())

	puede := actor.Role == user.RoleAdmin || actor.ID == autor
	if !puede {
		// Un profesor puede moderar su propio curso.
		if _, err := h.deps.Courses.GetCourse(r.Context(), actor, cursoID); err == nil {
			puede = true
		}
	}
	if !puede {
		writeError(w, ErrForbidden)
		return
	}

	if _, err := h.deps.Foros.EliminarRespuesta(r.Context(), postID); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type bloquearHiloRequest struct {
	Locked bool `json:"locked"`
}

// bloquearHilo cierra o reabre una conversación. Solo el profesor del curso.
func (h *handlers) bloquearHilo(w http.ResponseWriter, r *http.Request) {
	threadID, err := uuid.Parse(r.PathValue("threadId"))
	if err != nil {
		writeError(w, ErrBadRequest)
		return
	}
	hilo, err := h.deps.Foros.Hilo(r.Context(), threadID)
	if err != nil {
		writeError(w, err)
		return
	}
	actor, _ := UserFromContext(r.Context())
	if _, err := h.deps.Courses.GetCourse(r.Context(), actor, hilo.CourseID); err != nil {
		writeError(w, err)
		return
	}

	var req bloquearHiloRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if err := h.deps.Foros.BloquearHilo(r.Context(), threadID, req.Locked); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"locked": req.Locked})
}
