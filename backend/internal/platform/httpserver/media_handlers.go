package httpserver

import (
	"context"
	"io"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/DES-SOLUCIONES-CLOUD/proyecto-1/backend/internal/platform/storage"
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

// AlmacenDeCargas es lo que la autoría necesita del almacenamiento para subir
// material: firmar, cerrar y auditar una carga.
//
// Va estrecha y en forma de interfaz por la misma razón que EntregaDeObjetos,
// y por una más: el cliente real contacta al almacén al construirse, así que
// sin esto los handlers de carga no se pueden probar sin un MinIO vivo —y lo
// que no se puede probar acaba roto.
type AlmacenDeCargas interface {
	PresignedPutURL(ctx context.Context, objectKey string, expiry time.Duration) (string, error)
	InitiateMultipartUpload(ctx context.Context, objectKey, contentType string) (string, error)
	PresignedUploadPartURL(ctx context.Context, objectKey, uploadID string, partNumber int, expiry time.Duration) (string, error)
	CerrarCargaMultiparte(ctx context.Context, objectKey, uploadID string, partes []storage.ParteCargada) error
	PartesYaSubidas(ctx context.Context, objectKey, uploadID string) ([]storage.ParteEnCurso, error)
	Metadatos(ctx context.Context, objectKey string) (storage.ObjetoInfo, error)
	DetectMIME(ctx context.Context, objectKey string) (string, error)
	CalculateSHA256(ctx context.Context, objectKey string) (string, error)
	AbrirObjeto(ctx context.Context, objectKey string) (io.ReadCloser, error)
	RemoveObject(ctx context.Context, objectKey string) error
}

func (h *handlers) registerMedia(mux *http.ServeMux) {
	mux.Handle("GET /api/v1/resources/{resourceId}/content",
		h.auth()(http.HandlerFunc(h.resourceContent)))
	mux.Handle("PUT /api/v1/resources/{resourceId}/position",
		h.auth()(http.HandlerFunc(h.saveResourcePosition)))
}

type contentResponse struct {
	Type         string `json:"type"`
	Title        string `json:"title"`
	Downloadable bool   `json:"downloadable"`
	// URL del objeto: la lista maestra HLS en video y audio, el archivo en
	// PDF, imagen y descargables.
	URL string `json:"url,omitempty"`
	// CDN indica si la URL viene de una base pública y por tanto no caduca.
	CDN bool `json:"cdn,omitempty"`
	// ExpiresIn son los segundos de validez cuando la URL va firmada.
	ExpiresIn int `json:"expires_in,omitempty"`
	// Markdown es el contenido de los recursos de texto.
	Markdown string `json:"markdown,omitempty"`
	// ExternalURL es el destino de enlaces e iframes.
	ExternalURL string `json:"external_url,omitempty"`
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
		Type: contenido.Tipo, Title: contenido.Titulo,
		Downloadable: contenido.Descargable, Markdown: contenido.Markdown,
		ExternalURL: contenido.URLExterna, PositionSeconds: contenido.PosicionSegundos,
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
