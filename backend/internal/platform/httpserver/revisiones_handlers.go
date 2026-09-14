package httpserver

import (
	"context"
	"errors"
	"net/http"
	"strconv"

	"github.com/google/uuid"

	domain "github.com/DES-SOLUCIONES-CLOUD/proyecto-1/backend/internal/domain/course"
	"github.com/DES-SOLUCIONES-CLOUD/proyecto-1/backend/internal/domain/user"
	"github.com/DES-SOLUCIONES-CLOUD/proyecto-1/backend/internal/platform/postgres"
)

// Revisiones es el historial de contenido de un recurso.
type Revisiones interface {
	Guardar(ctx context.Context, recursoID uuid.UUID, contenido string, autorID uuid.UUID, autorEmail string) (*postgres.Revision, error)
	Listar(ctx context.Context, recursoID uuid.UUID, limite int) ([]postgres.Revision, error)
	PorNumero(ctx context.Context, recursoID uuid.UUID, numero int) (*postgres.Revision, error)
}

func (h *handlers) registerRevisiones(mux *http.ServeMux) {
	teacherOrAdmin := RequireRole(user.RoleTeacher, user.RoleAdmin)
	base := "/api/v1/courses/versions/{versionId}/resources/{resourceId}/revisions"

	mux.Handle("GET "+base, h.auth()(teacherOrAdmin(http.HandlerFunc(h.listarRevisiones))))
	mux.Handle("POST "+base, h.auth()(teacherOrAdmin(http.HandlerFunc(h.guardarRevision))))
	mux.Handle("GET "+base+"/{numero}", h.auth()(teacherOrAdmin(http.HandlerFunc(h.verRevision))))
	mux.Handle("POST "+base+"/{numero}/restore", h.auth()(teacherOrAdmin(http.HandlerFunc(h.restaurarRevision))))
}

// recursoEditable resuelve el recurso comprobando que quien pide puede
// editarlo. Se usa en las cuatro operaciones del historial: el historial de un
// curso ajeno no se lee ni se escribe.
func (h *handlers) recursoEditable(r *http.Request) (*domain.Resource, uuid.UUID, error) {
	versionID, err1 := uuid.Parse(r.PathValue("versionId"))
	resourceID, err2 := uuid.Parse(r.PathValue("resourceId"))
	if err1 != nil || err2 != nil {
		return nil, uuid.Nil, ErrBadRequest
	}
	actor, _ := UserFromContext(r.Context())
	res, err := h.deps.Courses.GetResource(r.Context(), actor, versionID, resourceID)
	if err != nil {
		return nil, uuid.Nil, err
	}
	return res, versionID, nil
}

func (h *handlers) listarRevisiones(w http.ResponseWriter, r *http.Request) {
	res, _, err := h.recursoEditable(r)
	if err != nil {
		writeError(w, err)
		return
	}
	revisiones, err := h.deps.Revisiones.Listar(r.Context(), res.ID, 0)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": revisiones})
}

type guardarRevisionRequest struct {
	ContentMD string `json:"content_md"`
}

// guardarRevision deja constancia de un guardado del editor.
//
// Devuelve 200 y no 201 cuando el contenido no cambió: repetir el
// autoguardado sin haber escrito nada no es un error del cliente, es lo normal,
// y responderle un error le haría enseñar una alerta cada pocos segundos.
func (h *handlers) guardarRevision(w http.ResponseWriter, r *http.Request) {
	res, versionID, err := h.recursoEditable(r)
	if err != nil {
		writeError(w, err)
		return
	}
	var req guardarRevisionRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	actor, _ := UserFromContext(r.Context())

	// El contenido vigente del recurso se actualiza junto con la revisión: el
	// historial acompaña al contenido, no lo sustituye.
	res.TextContentMD = req.ContentMD
	if err := h.deps.Courses.UpdateResource(r.Context(), actor, versionID, res); err != nil {
		writeError(w, err)
		return
	}

	rev, err := h.deps.Revisiones.Guardar(r.Context(), res.ID, req.ContentMD, actor.ID, actor.Email)
	if errors.Is(err, postgres.ErrSinCambios) {
		writeJSON(w, http.StatusOK, map[string]any{"status": "sin_cambios"})
		return
	}
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, rev)
}

func (h *handlers) verRevision(w http.ResponseWriter, r *http.Request) {
	res, _, err := h.recursoEditable(r)
	if err != nil {
		writeError(w, err)
		return
	}
	numero, err := numeroDeRevision(r)
	if err != nil {
		writeError(w, err)
		return
	}
	rev, err := h.deps.Revisiones.PorNumero(r.Context(), res.ID, numero)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, rev)
}

// restaurarRevision devuelve el contenido de una revisión anterior al recurso.
//
// No borra nada ni "vuelve atrás" en el historial: la restauración se guarda
// como una revisión nueva. Así queda constancia de que alguien restauró, y
// deshacer una restauración equivocada es otra restauración, no una pérdida.
func (h *handlers) restaurarRevision(w http.ResponseWriter, r *http.Request) {
	res, versionID, err := h.recursoEditable(r)
	if err != nil {
		writeError(w, err)
		return
	}
	numero, err := numeroDeRevision(r)
	if err != nil {
		writeError(w, err)
		return
	}
	actor, _ := UserFromContext(r.Context())

	anterior, err := h.deps.Revisiones.PorNumero(r.Context(), res.ID, numero)
	if err != nil {
		writeError(w, err)
		return
	}

	res.TextContentMD = anterior.ContenidoMD
	if err := h.deps.Courses.UpdateResource(r.Context(), actor, versionID, res); err != nil {
		writeError(w, err)
		return
	}

	rev, err := h.deps.Revisiones.Guardar(r.Context(), res.ID, anterior.ContenidoMD, actor.ID, actor.Email)
	if errors.Is(err, postgres.ErrSinCambios) {
		// Restaurar lo que ya estaba puesto no añade nada al historial.
		writeJSON(w, http.StatusOK, map[string]any{"status": "sin_cambios"})
		return
	}
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"status": "restaurada", "restored_from": numero, "revision": rev,
	})
}

func numeroDeRevision(r *http.Request) (int, error) {
	n, err := strconv.Atoi(r.PathValue("numero"))
	if err != nil || n <= 0 {
		return 0, ErrBadRequest
	}
	return n, nil
}
