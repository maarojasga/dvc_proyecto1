package httpserver

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/DES-SOLUCIONES-CLOUD/proyecto-1/backend/internal/platform/postgres"
)

// Exportacion reúne los datos personales de una cuenta.
type Exportacion interface {
	DeUsuario(ctx context.Context, userID uuid.UUID) (*postgres.DatosPersonales, error)
}

func (h *handlers) registerExportacion(mux *http.ServeMux) {
	// Sin restricción de rol: el derecho a llevarse los propios datos no
	// depende de para qué se use la plataforma. Cada quien exporta los suyos y
	// solo los suyos, porque el identificador sale de la sesión y no de la
	// petición.
	mux.Handle("GET /api/v1/me/export", h.auth()(http.HandlerFunc(h.exportarMisDatos)))
}

// exportarMisDatos entrega todo lo que la plataforma guarda sobre quien lo
// pide, como un archivo descargable.
//
// Va como descarga y no como respuesta a leer en pantalla porque eso es lo que
// se hace con una exportación: guardarla o llevarla a otro sitio.
func (h *handlers) exportarMisDatos(w http.ResponseWriter, r *http.Request) {
	if h.deps.Exportacion == nil {
		writeError(w, ErrBadRequest)
		return
	}
	actor, _ := UserFromContext(r.Context())

	datos, err := h.deps.Exportacion.DeUsuario(r.Context(), actor.ID)
	if err != nil {
		writeError(w, err)
		return
	}

	nombre := fmt.Sprintf("mis-datos-%s.json", time.Now().UTC().Format("2006-01-02"))
	w.Header().Set("Content-Disposition", `attachment; filename="`+nombre+`"`)
	// La exportación no se cachea: es un documento con datos personales y no
	// debería quedarse en ninguna caché intermedia.
	w.Header().Set("Cache-Control", "no-store")

	h.auditar(r, actor.ID, "user.data_exported", "user", actor.ID.String(), nil)
	writeJSON(w, http.StatusOK, datos)
}
