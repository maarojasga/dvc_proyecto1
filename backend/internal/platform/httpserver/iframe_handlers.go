package httpserver

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/google/uuid"

	"github.com/DES-SOLUCIONES-CLOUD/proyecto-1/backend/internal/domain/iframe"
	"github.com/DES-SOLUCIONES-CLOUD/proyecto-1/backend/internal/domain/user"
	"github.com/DES-SOLUCIONES-CLOUD/proyecto-1/backend/internal/platform/postgres"
)

// ListaBlancaDeIframes es lo que la API necesita para administrar los destinos
// incrustables.
type ListaBlancaDeIframes interface {
	Listar(ctx context.Context) (iframe.Lista, error)
	Agregar(ctx context.Context, d *iframe.Destino, actorID uuid.UUID) error
	Eliminar(ctx context.Context, id uuid.UUID) error
	PorID(ctx context.Context, id uuid.UUID) (*iframe.Destino, error)
}

func (h *handlers) registerIframes(mux *http.ServeMux) {
	admin := RequireRole(user.RoleAdmin)
	teacherOrAdmin := RequireRole(user.RoleTeacher, user.RoleAdmin)

	// La consulta la necesita también el profesor: al añadir un recurso
	// incrustado tiene que poder ver qué destinos se admiten, en vez de
	// descubrirlo por ensayo y error contra un 422.
	mux.Handle("GET /api/v1/iframe-allowlist",
		h.auth()(teacherOrAdmin(http.HandlerFunc(h.listarDestinosIframe))))
	mux.Handle("POST /api/v1/admin/iframe-allowlist",
		h.auth()(admin(http.HandlerFunc(h.agregarDestinoIframe))))
	mux.Handle("DELETE /api/v1/admin/iframe-allowlist/{id}",
		h.auth()(admin(http.HandlerFunc(h.eliminarDestinoIframe))))
}

type destinoIframeResponse struct {
	ID                 uuid.UUID `json:"id"`
	Host               string    `json:"host"`
	IncludeSubdomains  bool      `json:"include_subdomains"`
	Permissions        string    `json:"permissions,omitempty"`
	Description        string    `json:"description,omitempty"`
	EjemploDeIncrustar string    `json:"example_url,omitempty"`
}

func (h *handlers) listarDestinosIframe(w http.ResponseWriter, r *http.Request) {
	if h.deps.Iframes == nil {
		writeError(w, ErrBadRequest)
		return
	}
	lista, err := h.deps.Iframes.Listar(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	items := make([]destinoIframeResponse, 0, len(lista))
	for _, d := range lista {
		items = append(items, destinoIframeResponse{
			ID: d.ID, Host: d.Host, IncludeSubdomains: d.IncluirSubdominios,
			Permissions: d.Permisos, Description: d.Descripcion,
			EjemploDeIncrustar: "https://" + d.Host + "/",
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"items": items,
		// El sandbox es el mismo para todos los destinos, así que viaja una
		// vez y no repetido en cada entrada.
		"sandbox":         iframe.Sandbox(),
		"referrer_policy": iframe.ReferrerPolicy,
	})
}

type agregarDestinoRequest struct {
	Host              string `json:"host"`
	IncludeSubdomains bool   `json:"include_subdomains"`
	Permissions       string `json:"permissions"`
	Description       string `json:"description"`
}

func (h *handlers) agregarDestinoIframe(w http.ResponseWriter, r *http.Request) {
	var req agregarDestinoRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	host, err := iframe.NormalizarHost(req.Host)
	if err != nil {
		writeError(w, err)
		return
	}
	actor, _ := UserFromContext(r.Context())

	d := iframe.Destino{
		Host: host, IncluirSubdominios: req.IncludeSubdomains,
		Permisos: req.Permissions, Descripcion: req.Description,
	}
	if err := h.deps.Iframes.Agregar(r.Context(), &d, actor.ID); err != nil {
		writeError(w, err)
		return
	}
	h.auditar(r, actor.ID, "iframe_allowlist.added", "iframe_allowlist", d.ID.String(), map[string]string{
		"host": d.Host, "subdominios": boolATexto(d.IncluirSubdominios), "permisos": d.Permisos,
	})
	writeJSON(w, http.StatusCreated, destinoIframeResponse{
		ID: d.ID, Host: d.Host, IncludeSubdomains: d.IncluirSubdominios,
		Permissions: d.Permisos, Description: d.Descripcion,
	})
}

func (h *handlers) eliminarDestinoIframe(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeError(w, ErrBadRequest)
		return
	}
	// Se lee antes de borrar para dejar en la bitácora qué host salió: un
	// registro que solo guarde el identificador no dice nada al revisarlo.
	d, err := h.deps.Iframes.PorID(r.Context(), id)
	if err != nil {
		writeError(w, err)
		return
	}
	if err := h.deps.Iframes.Eliminar(r.Context(), id); err != nil {
		writeError(w, err)
		return
	}
	actor, _ := UserFromContext(r.Context())
	h.auditar(r, actor.ID, "iframe_allowlist.removed", "iframe_allowlist", id.String(), map[string]string{
		"host": d.Host,
	})
	// Los recursos ya publicados que apunten a este host dejan de entregarse
	// en la siguiente petición: la autorización se rehace al servir.
	w.WriteHeader(http.StatusNoContent)
}

func boolATexto(b bool) string {
	if b {
		return "sí"
	}
	return "no"
}

// auditar deja constancia de una acción administrativa sin interrumpirla si la
// bitácora falla.
func (h *handlers) auditar(r *http.Request, actorID uuid.UUID, accion, entidad, entidadID string, detalle map[string]string) {
	if h.deps.Auditor == nil {
		return
	}
	meta, err := json.Marshal(detalle)
	if err != nil {
		return
	}
	_ = h.deps.Auditor.InsertAudit(r.Context(), postgres.AuditEntry{
		ActorUserID: &actorID, Action: accion, EntityType: entidad,
		EntityID: entidadID, Metadata: meta, IPAddress: clientIP(r),
	})
}
