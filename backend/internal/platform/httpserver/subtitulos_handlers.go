package httpserver

import (
	"context"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"github.com/DES-SOLUCIONES-CLOUD/proyecto-1/backend/internal/domain/subtitulo"
	"github.com/DES-SOLUCIONES-CLOUD/proyecto-1/backend/internal/domain/user"
	"github.com/DES-SOLUCIONES-CLOUD/proyecto-1/backend/internal/platform/postgres"
)

// Subtitulos es el almacén de pistas y transcripciones.
type Subtitulos interface {
	Guardar(ctx context.Context, s *postgres.Subtitulo) error
	Listar(ctx context.Context, resourceID uuid.UUID) ([]postgres.Subtitulo, error)
	PorIdioma(ctx context.Context, resourceID uuid.UUID, idioma string) (*postgres.Subtitulo, error)
	Eliminar(ctx context.Context, resourceID uuid.UUID, idioma string) error
}

func (h *handlers) registerSubtitulos(mux *http.ServeMux) {
	teacherOrAdmin := RequireRole(user.RoleTeacher, user.RoleAdmin)
	autoria := "/api/v1/courses/versions/{versionId}/resources/{resourceId}/captions"

	mux.Handle("PUT "+autoria+"/{idioma}", h.auth()(teacherOrAdmin(http.HandlerFunc(h.guardarSubtitulo))))
	mux.Handle("DELETE "+autoria+"/{idioma}", h.auth()(teacherOrAdmin(http.HandlerFunc(h.eliminarSubtitulo))))

	// La lectura la hace el estudiante desde el reproductor, así que cuelga
	// del recurso publicado y no de la versión en autoría.
	mux.Handle("GET /api/v1/resources/{resourceId}/captions",
		h.auth()(http.HandlerFunc(h.listarSubtitulos)))
	mux.Handle("GET /api/v1/resources/{resourceId}/transcript/{idioma}",
		h.auth()(http.HandlerFunc(h.verTranscripcion)))
}

// tamanoMaximoDeVTT acota lo que se acepta como pista. Una hora de subtítulos
// densos no llega a 200 KB; el límite está para que un archivo enorme no se
// cargue entero en memoria.
const tamanoMaximoDeVTT = 2 << 20 // 2 MiB

type guardarSubtituloRequest struct {
	Etiqueta string `json:"label"`
	Tipo     string `json:"kind"`
	// ContenidoVTT es el archivo completo. Va en el cuerpo JSON y no como
	// carga directa al almacén, al contrario que el vídeo, porque el servidor
	// necesita leerlo para derivar la transcripción: una pista que el
	// navegador sube por su cuenta llegaría sin transcripción, y pedirla
	// aparte garantizaría que las dos acaben diciendo cosas distintas.
	ContenidoVTT string `json:"vtt"`
}

func (h *handlers) guardarSubtitulo(w http.ResponseWriter, r *http.Request) {
	res, _, err := h.recursoEditable(r)
	if err != nil {
		writeError(w, err)
		return
	}
	idioma := normalizarIdioma(r.PathValue("idioma"))
	if idioma == "" {
		writeError(w, ErrBadRequest)
		return
	}

	var req guardarSubtituloRequest
	r.Body = http.MaxBytesReader(w, r.Body, tamanoMaximoDeVTT)
	if !decodeJSON(w, r, &req) {
		return
	}

	pista, err := subtitulo.Analizar(req.ContenidoVTT)
	if err != nil {
		writeError(w, err)
		return
	}

	clave := claveDeSubtitulo(res.ID, idioma)
	if h.deps.Storage != nil {
		if err := h.deps.Storage.GuardarObjeto(r.Context(), clave,
			strings.NewReader(req.ContenidoVTT), int64(len(req.ContenidoVTT)), "text/vtt"); err != nil {
			writeError(w, err)
			return
		}
	}

	tipo := req.Tipo
	if tipo != "captions" {
		tipo = "subtitles"
	}
	etiqueta := req.Etiqueta
	if etiqueta == "" {
		etiqueta = idioma
	}

	s := postgres.Subtitulo{
		ResourceID: res.ID, Idioma: idioma, Etiqueta: etiqueta,
		ObjectKey: clave, Transcripcion: pista.Transcripcion(), Tipo: tipo,
	}
	if err := h.deps.Subtitulos.Guardar(r.Context(), &s); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"language": s.Idioma, "label": s.Etiqueta, "kind": s.Tipo,
		"cues": len(pista), "duration_ms": pista.Duracion(),
	})
}

func (h *handlers) eliminarSubtitulo(w http.ResponseWriter, r *http.Request) {
	res, _, err := h.recursoEditable(r)
	if err != nil {
		writeError(w, err)
		return
	}
	idioma := normalizarIdioma(r.PathValue("idioma"))
	if err := h.deps.Subtitulos.Eliminar(r.Context(), res.ID, idioma); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type subtituloResponse struct {
	Idioma   string `json:"language"`
	Etiqueta string `json:"label"`
	Tipo     string `json:"kind"`
	// URL es la del archivo WebVTT, firmada o servida por el CDN. Es lo que el
	// reproductor pone en <track src>.
	URL string `json:"url,omitempty"`
}

// listarSubtitulos entrega las pistas de un recurso al reproductor.
//
// Autoriza por la misma vía que el contenido: si el recurso no se le puede
// entregar a quien pregunta, sus subtítulos tampoco.
func (h *handlers) listarSubtitulos(w http.ResponseWriter, r *http.Request) {
	resourceID, err := uuid.Parse(r.PathValue("resourceId"))
	if err != nil {
		writeError(w, ErrBadRequest)
		return
	}
	actor, _ := UserFromContext(r.Context())
	if _, err := h.deps.Enrollments.ContenidoDeRecurso(r.Context(), actor, resourceID); err != nil {
		writeError(w, err)
		return
	}

	pistas, err := h.deps.Subtitulos.Listar(r.Context(), resourceID)
	if err != nil {
		writeError(w, err)
		return
	}
	items := make([]subtituloResponse, 0, len(pistas))
	for _, p := range pistas {
		item := subtituloResponse{Idioma: p.Idioma, Etiqueta: p.Etiqueta, Tipo: p.Tipo}
		if h.deps.Entrega != nil {
			if url, err := h.deps.Entrega.PresignedGetURL(r.Context(), p.ObjectKey, vigenciaEntrega, ""); err == nil {
				item.URL = url
			}
		}
		items = append(items, item)
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

// verTranscripcion entrega el texto corrido de una pista.
//
// Es lo que hace el material legible para quien no puede o no quiere
// reproducirlo, y lo que permite buscar dentro de una clase grabada.
func (h *handlers) verTranscripcion(w http.ResponseWriter, r *http.Request) {
	resourceID, err := uuid.Parse(r.PathValue("resourceId"))
	if err != nil {
		writeError(w, ErrBadRequest)
		return
	}
	actor, _ := UserFromContext(r.Context())
	contenido, err := h.deps.Enrollments.ContenidoDeRecurso(r.Context(), actor, resourceID)
	if err != nil {
		writeError(w, err)
		return
	}

	pista, err := h.deps.Subtitulos.PorIdioma(r.Context(), resourceID, normalizarIdioma(r.PathValue("idioma")))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"resource_title": contenido.Titulo,
		"language":       pista.Idioma,
		"transcript":     pista.Transcripcion,
	})
}

func claveDeSubtitulo(resourceID uuid.UUID, idioma string) string {
	return "subtitulos/" + resourceID.String() + "/" + idioma + ".vtt"
}

// normalizarIdioma acepta etiquetas BCP 47 sencillas ("es", "pt-BR") y
// descarta cualquier cosa que pudiera acabar formando parte de una clave de
// objeto sin control.
func normalizarIdioma(crudo string) string {
	s := strings.ToLower(strings.TrimSpace(crudo))
	if s == "" || len(s) > 12 {
		return ""
	}
	for _, c := range s {
		if !(c >= 'a' && c <= 'z') && c != '-' {
			return ""
		}
	}
	return s
}
