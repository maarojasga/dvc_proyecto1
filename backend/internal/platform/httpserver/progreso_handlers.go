package httpserver

import (
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/DES-SOLUCIONES-CLOUD/proyecto-1/backend/internal/domain/user"
)

func (h *handlers) registerProgress(mux *http.ServeMux) {
	student := RequireRole(user.RoleStudent)
	admin := RequireRole(user.RoleAdmin)

	mux.Handle("POST /api/v1/resources/{resourceId}/progress",
		h.auth()(student(http.HandlerFunc(h.recordProgressEvent))))
	// Va bajo /enrollments y no bajo /courses/{courseId}/...: ese prefijo ya
	// tiene /courses/versions/{versionId} registrado, y un comodin compitiendo
	// con un literal en la misma posicion hace que net/http.ServeMux rechace
	// el patron como ambiguo en tiempo de arranque.
	mux.Handle("GET /api/v1/enrollments/{courseId}/progress",
		h.auth()(student(http.HandlerFunc(h.getCourseProgress))))
	mux.Handle("GET /api/v1/badges/mine",
		h.auth()(student(http.HandlerFunc(h.listMyBadges))))
	mux.Handle("POST /api/v1/badges/{code}/revoke",
		h.auth()(admin(http.HandlerFunc(h.revokeBadge))))

	// La verificacion es publica y sin sesion: ese es justamente el punto de
	// una insignia verificable.
	mux.Handle("GET /api/v1/badges/{code}", http.HandlerFunc(h.verifyBadge))
}

type progressEventRequest struct {
	Type     string `json:"type"`
	Complete bool   `json:"complete"`
}

func (h *handlers) recordProgressEvent(w http.ResponseWriter, r *http.Request) {
	resourceID, err := uuid.Parse(r.PathValue("resourceId"))
	if err != nil {
		writeError(w, ErrBadRequest)
		return
	}
	var req progressEventRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	actor, _ := UserFromContext(r.Context())
	res, err := h.deps.Progreso.RegistrarEvento(r.Context(), actor, resourceID, req.Type, req.Complete)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (h *handlers) getCourseProgress(w http.ResponseWriter, r *http.Request) {
	courseID, err := uuid.Parse(r.PathValue("courseId"))
	if err != nil {
		writeError(w, ErrBadRequest)
		return
	}
	actor, _ := UserFromContext(r.Context())
	res, err := h.deps.Progreso.Resumen(r.Context(), actor, courseID)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

// insigniaResponse es la insignia tal como la ve su dueño, con la URL de la
// imagen ya resuelta.
type insigniaResponse struct {
	Code      string     `json:"code"`
	CourseID  uuid.UUID  `json:"course_id"`
	Valid     bool       `json:"valid"`
	IssuedAt  time.Time  `json:"issued_at"`
	RevokedAt *time.Time `json:"revoked_at,omitempty"`
	ImageURL  string     `json:"image_url,omitempty"`
	// VerifyPath es la ruta pública de verificación, para que el cliente
	// construya el enlace que se comparte sin inventarse el formato.
	VerifyPath string `json:"verify_path"`
}

func (h *handlers) listMyBadges(w http.ResponseWriter, r *http.Request) {
	actor, _ := UserFromContext(r.Context())
	list, err := h.deps.Progreso.MisInsignias(r.Context(), actor)
	if err != nil {
		writeError(w, err)
		return
	}
	items := make([]insigniaResponse, 0, len(list))
	for _, b := range list {
		items = append(items, insigniaResponse{
			Code: b.VerificationCode, CourseID: b.CourseID, Valid: b.Verified(),
			IssuedAt: b.IssuedAt, RevokedAt: b.RevokedAt,
			ImageURL:   h.urlDeImagen(r, b.ImageObjectKey),
			VerifyPath: "/api/v1/badges/" + b.VerificationCode,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

// verificacionResponse es la vista pública. No lleva nada del estudiante: esa
// es la condición de aceptación de la emisión de insignias.
type verificacionResponse struct {
	Code      string     `json:"code"`
	CourseID  uuid.UUID  `json:"course_id"`
	Valid     bool       `json:"valid"`
	IssuedAt  time.Time  `json:"issued_at"`
	RevokedAt *time.Time `json:"revoked_at,omitempty"`
	ImageURL  string     `json:"image_url,omitempty"`
}

func (h *handlers) verifyBadge(w http.ResponseWriter, r *http.Request) {
	v, err := h.deps.Progreso.Verificar(r.Context(), r.PathValue("code"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, verificacionResponse{
		Code: v.Code, CourseID: v.CourseID, Valid: v.Valid,
		IssuedAt: v.IssuedAt, RevokedAt: v.RevokedAt,
		ImageURL: h.urlDeImagen(r, v.ImagenClave),
	})
}

// urlDeImagen resuelve dónde está la imagen de la insignia.
//
// La imagen no es material privado de un curso —su URL se comparte junto con
// la verificación—, pero vive en el mismo almacén, así que sale por el CDN
// cuando lo hay y firmada cuando no. Si el objeto no se llegó a escribir, se
// devuelve vacío y el cliente muestra la insignia sin imagen.
func (h *handlers) urlDeImagen(r *http.Request, clave string) string {
	if clave == "" || h.deps.Entrega == nil {
		return ""
	}
	url, err := h.deps.Entrega.PresignedGetURL(r.Context(), clave, vigenciaEntrega, "")
	if err != nil {
		return ""
	}
	return url
}

type revokeBadgeRequest struct {
	Reason string `json:"reason"`
}

func (h *handlers) revokeBadge(w http.ResponseWriter, r *http.Request) {
	var req revokeBadgeRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	actor, _ := UserFromContext(r.Context())
	if err := h.deps.Progreso.Revocar(r.Context(), actor, r.PathValue("code"), req.Reason); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "revoked"})
}
