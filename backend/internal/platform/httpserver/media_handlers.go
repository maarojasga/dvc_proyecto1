package httpserver

import (
	"context"
	"net/http"
	"time"

	"github.com/google/uuid"
)

// EntregaDeObjetos es lo único que la reproducción necesita del
// almacenamiento: resolver la URL de lectura de un objeto ya autorizado.
//
// Se declara aquí, estrecha, en lugar de depender del cliente completo: así
// una instancia que solo entrega contenido, o una prueba que verifica la
// autorización, no arrastran la capacidad de firmar subidas.
type EntregaDeObjetos interface {
	PresignedGetURL(ctx context.Context, objectKey string, expiry time.Duration, filename string) (string, error)
	SirveDesdeCDN() bool
}

// vigenciaReproduccion es lo que dura la URL entregada al reproductor. Corta
// a propósito: el manifiesto se pide al empezar, no se guarda.
const vigenciaReproduccion = 15 * time.Minute

func (h *handlers) registerMedia(mux *http.ServeMux) {
	mux.Handle("GET /api/v1/resources/{resourceId}/playback",
		h.auth()(http.HandlerFunc(h.resourcePlayback)))
}

// resourcePlayback entrega la URL de la lista maestra HLS de un recurso.
//
// La autorización ocurre antes de firmar nada: el recurso debe pertenecer a
// la versión vigente del curso, ser visible y quien lo pide debe estar
// inscrito (el profesor dueño y la administración pasan sin inscripción).
//
// La URL sale del CDN cuando S3_PUBLIC_URL está configurado, y de una firma
// temporal en caso contrario. Nota de despliegue: el reproductor pide los
// segmentos con rutas relativas al manifiesto, así que la autorización de
// esos objetos la resuelve el CDN; sin CDN delante, el prefijo hls/ debe
// servirse de forma que el reproductor pueda leerlo.
func (h *handlers) resourcePlayback(w http.ResponseWriter, r *http.Request) {
	resourceID, err := uuid.Parse(r.PathValue("resourceId"))
	if err != nil {
		writeError(w, ErrBadRequest)
		return
	}
	actor, _ := UserFromContext(r.Context())

	clave, err := h.deps.Enrollments.ClaveDeReproduccion(r.Context(), actor, resourceID)
	if err != nil {
		writeError(w, err)
		return
	}

	url, err := h.deps.Entrega.PresignedGetURL(r.Context(), clave, vigenciaReproduccion, "")
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"url":        url,
		"type":       "application/vnd.apple.mpegurl",
		"expires_in": int(vigenciaReproduccion.Seconds()),
		"cdn":        h.deps.Entrega.SirveDesdeCDN(),
	})
}
