package httpserver

import (
	"context"
	"net/http"
	"time"

	"github.com/google/uuid"
)

// vigenciaEntrega es lo que dura una URL firmada de contenido. Corta a
// propósito: se pide al abrir el recurso, no se guarda.
const vigenciaEntrega = 15 * time.Minute

// EntregaDeObjetos es lo único que la entrega de contenido necesita del
// almacenamiento: resolver la URL de lectura de un objeto ya autorizado.
//
// Se declara aquí, estrecha, en lugar de depender del cliente completo: así
// una instancia que solo entrega contenido, o una prueba que verifica la
// autorización, no arrastran la capacidad de firmar subidas.
type EntregaDeObjetos interface {
	PresignedGetURL(ctx context.Context, objectKey string, expiry time.Duration, filename string) (string, error)
	SirveDesdeCDN() bool
}

func (h *handlers) registerMedia(mux *http.ServeMux) {
	mux.Handle("GET /api/v1/resources/{resourceId}/content",
		h.auth()(http.HandlerFunc(h.resourceContent)))
	mux.Handle("PUT /api/v1/resources/{resourceId}/position",
		h.auth()(http.HandlerFunc(h.saveResourcePosition)))
}

type contentResponse struct {
	Type string `json:"type"`
	// StableID identifica el recurso a través de las versiones. Lo usa el foro
	// para atar una conversación a la lección y no a la fila.
	StableID     string `json:"stable_id"`
	Title        string `json:"title"`
	Downloadable bool   `json:"downloadable"`
	// URL del objeto: la lista maestra HLS en video y audio, el archivo en
	// PDF, imagen y descargables.
	URL string `json:"url,omitempty"`
	// CDN indica si la URL viene de una base pública y por tanto no caduca.
	CDN bool `json:"cdn,omitempty"`
	// ExpiresIn son los segundos de validez cuando la URL va firmada.
	ExpiresIn int `json:"expires_in,omitempty"`
	// OriginalURL es el archivo tal como lo subió el profesor, cuando el
	// recurso se presenta convertido (una presentación) y además es
	// descargable.
	OriginalURL string `json:"original_url,omitempty"`
	// Markdown es el contenido de los recursos de texto.
	Markdown string `json:"markdown,omitempty"`
	// ExternalURL es el destino de enlaces e iframes.
	ExternalURL string `json:"external_url,omitempty"`
	// Sandbox, Allow y ReferrerPolicy son los atributos con los que el cliente
	// debe montar un iframe. Los decide el servidor: si el cliente eligiera
	// cuánto restringir el contenido de terceros, bastaría con manipularlo
	// para que no restringiera nada.
	Sandbox        string `json:"sandbox,omitempty"`
	Allow          string `json:"allow,omitempty"`
	ReferrerPolicy string `json:"referrer_policy,omitempty"`
	// PositionSeconds es dónde reanudar la reproducción.
	PositionSeconds int `json:"position_seconds,omitempty"`
}

// resourceContent entrega lo necesario para presentar un recurso.
//
// La autorización ocurre antes de resolver ninguna URL: el recurso debe
// pertenecer a la versión vigente del curso, ser visible y quien lo pide debe
// estar inscrito (el profesor dueño y la administración pasan sin
// inscripción).
//
// La URL de los objetos sale del CDN cuando S3_PUBLIC_URL está configurado, y
// de una firma temporal en caso contrario. Nota de despliegue: el reproductor
// pide los segmentos HLS con rutas relativas al manifiesto, así que la
// autorización de esos objetos la resuelve el CDN.
func (h *handlers) resourceContent(w http.ResponseWriter, r *http.Request) {
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

	out := contentResponse{
		Type: contenido.Tipo, StableID: contenido.StableID.String(), Title: contenido.Titulo,
		Downloadable: contenido.Descargable, Markdown: contenido.Markdown,
		ExternalURL: contenido.URLExterna, PositionSeconds: contenido.PosicionSegundos,
		Sandbox: contenido.Sandbox, Allow: contenido.Permisos,
		ReferrerPolicy: contenido.ReferrerPolicy,
	}
	if contenido.ClaveObjeto != "" {
		url, err := h.deps.Entrega.PresignedGetURL(r.Context(), contenido.ClaveObjeto, vigenciaEntrega, "")
		if err != nil {
			writeError(w, err)
			return
		}
		out.URL = url
		out.CDN = h.deps.Entrega.SirveDesdeCDN()
		if !out.CDN {
			out.ExpiresIn = int(vigenciaEntrega.Seconds())
		}
	}
	if contenido.ClaveOriginal != "" {
		if url, err := h.deps.Entrega.PresignedGetURL(r.Context(), contenido.ClaveOriginal, vigenciaEntrega, ""); err == nil {
			out.OriginalURL = url
		}
	}
	writeJSON(w, http.StatusOK, out)
}

type positionRequest struct {
	PositionSeconds int `json:"position_seconds"`
}

// saveResourcePosition registra dónde reanudar la reproducción.
//
// Es una comodidad reportada por el cliente y no acredita avance: el progreso
// lo calcula el servidor por su cuenta a partir de heartbeats y eventos de
// apertura, así que lo que llegue aquí no puede sostener ninguna decisión
// académica.
func (h *handlers) saveResourcePosition(w http.ResponseWriter, r *http.Request) {
	resourceID, err := uuid.Parse(r.PathValue("resourceId"))
	if err != nil {
		writeError(w, ErrBadRequest)
		return
	}
	var req positionRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	actor, _ := UserFromContext(r.Context())

	if err := h.deps.Enrollments.GuardarPosicion(r.Context(), actor, resourceID, req.PositionSeconds); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
