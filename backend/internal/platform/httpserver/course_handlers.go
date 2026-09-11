package httpserver

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"

	domain "github.com/DES-SOLUCIONES-CLOUD/proyecto-1/backend/internal/domain/course"
	"github.com/DES-SOLUCIONES-CLOUD/proyecto-1/backend/internal/domain/user"
	"github.com/DES-SOLUCIONES-CLOUD/proyecto-1/backend/internal/platform/postgres"
	"github.com/DES-SOLUCIONES-CLOUD/proyecto-1/backend/internal/platform/queue"
	"github.com/DES-SOLUCIONES-CLOUD/proyecto-1/backend/internal/platform/storage"
)

func (h *handlers) registerCourses(mux *http.ServeMux) {
	teacherOrAdmin := RequireRole(user.RoleTeacher, user.RoleAdmin)

	mux.Handle("POST /api/v1/courses", h.auth()(teacherOrAdmin(http.HandlerFunc(h.createCourseDraft))))
	mux.Handle("GET /api/v1/courses/mine", h.auth()(teacherOrAdmin(http.HandlerFunc(h.listMyCourses))))
	mux.Handle("POST /api/v1/courses/{courseId}/update-draft", h.auth()(teacherOrAdmin(http.HandlerFunc(h.createUpdateDraft))))
	mux.Handle("POST /api/v1/courses/{courseId}/unpublish", h.auth()(teacherOrAdmin(http.HandlerFunc(h.unpublishCourse))))

	mux.Handle("GET /api/v1/courses/versions/{versionId}", h.auth()(teacherOrAdmin(http.HandlerFunc(h.previewVersion))))
	mux.Handle("PATCH /api/v1/courses/versions/{versionId}", h.auth()(teacherOrAdmin(http.HandlerFunc(h.updateVersionMetadata))))
	mux.Handle("POST /api/v1/courses/versions/{versionId}/publish", h.auth()(teacherOrAdmin(http.HandlerFunc(h.publishVersion))))

	mux.Handle("POST /api/v1/courses/versions/{versionId}/modules", h.auth()(teacherOrAdmin(http.HandlerFunc(h.addModule))))
	mux.Handle("PATCH /api/v1/courses/versions/{versionId}/modules/{moduleId}", h.auth()(teacherOrAdmin(http.HandlerFunc(h.updateModule))))
	mux.Handle("DELETE /api/v1/courses/versions/{versionId}/modules/{moduleId}", h.auth()(teacherOrAdmin(http.HandlerFunc(h.deleteModule))))

	mux.Handle("POST /api/v1/courses/versions/{versionId}/modules/{moduleId}/units", h.auth()(teacherOrAdmin(http.HandlerFunc(h.addUnit))))
	mux.Handle("PATCH /api/v1/courses/versions/{versionId}/units/{unitId}", h.auth()(teacherOrAdmin(http.HandlerFunc(h.updateUnit))))
	mux.Handle("DELETE /api/v1/courses/versions/{versionId}/units/{unitId}", h.auth()(teacherOrAdmin(http.HandlerFunc(h.deleteUnit))))

	mux.Handle("POST /api/v1/courses/versions/{versionId}/units/{unitId}/resources", h.auth()(teacherOrAdmin(http.HandlerFunc(h.addResource))))
	mux.Handle("PATCH /api/v1/courses/versions/{versionId}/resources/{resourceId}", h.auth()(teacherOrAdmin(http.HandlerFunc(h.updateResource))))
	mux.Handle("DELETE /api/v1/courses/versions/{versionId}/resources/{resourceId}", h.auth()(teacherOrAdmin(http.HandlerFunc(h.deleteResource))))
	mux.Handle("POST /api/v1/courses/versions/{versionId}/resources/{resourceId}/upload-url", h.auth()(teacherOrAdmin(http.HandlerFunc(h.requestResourceUploadURL))))
	mux.Handle("POST /api/v1/courses/versions/{versionId}/resources/{resourceId}/confirm-upload", h.auth()(teacherOrAdmin(http.HandlerFunc(h.confirmResourceUpload))))
	mux.Handle("POST /api/v1/courses/versions/{versionId}/resources/{resourceId}/multipart/initiate", h.auth()(teacherOrAdmin(http.HandlerFunc(h.initiateMultipartUpload))))
	mux.Handle("POST /api/v1/courses/versions/{versionId}/resources/{resourceId}/multipart/part-url", h.auth()(teacherOrAdmin(http.HandlerFunc(h.requestMultipartPartURL))))
	mux.Handle("POST /api/v1/courses/versions/{versionId}/resources/{resourceId}/multipart/complete", h.auth()(teacherOrAdmin(http.HandlerFunc(h.completeMultipartUpload))))
	mux.Handle("GET /api/v1/courses/versions/{versionId}/resources/{resourceId}/multipart/parts", h.auth()(teacherOrAdmin(http.HandlerFunc(h.listMultipartParts))))

	mux.HandleFunc("GET /api/v1/catalog", h.listCatalog)
	mux.HandleFunc("GET /api/v1/catalog/{courseId}", h.getPublishedCourse)
}

type createCourseRequest struct {
	Slug  string `json:"slug"`
	Title string `json:"title"`
}

func (h *handlers) createCourseDraft(w http.ResponseWriter, r *http.Request) {
	var req createCourseRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	actor, _ := UserFromContext(r.Context())
	c, v, err := h.deps.Courses.CreateDraft(r.Context(), actor, req.Slug, req.Title)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"course_id": c.ID, "version_id": v.ID, "version_number": v.VersionNumber})
}

func (h *handlers) listMyCourses(w http.ResponseWriter, r *http.Request) {
	actor, _ := UserFromContext(r.Context())
	list, err := h.deps.Courses.ListMine(r.Context(), actor)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": list})
}

func (h *handlers) createUpdateDraft(w http.ResponseWriter, r *http.Request) {
	courseID, err := uuid.Parse(r.PathValue("courseId"))
	if err != nil {
		writeError(w, ErrBadRequest)
		return
	}
	actor, _ := UserFromContext(r.Context())
	v, err := h.deps.Courses.CreateUpdateDraft(r.Context(), actor, courseID)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, v)
}

func (h *handlers) unpublishCourse(w http.ResponseWriter, r *http.Request) {
	courseID, err := uuid.Parse(r.PathValue("courseId"))
	if err != nil {
		writeError(w, ErrBadRequest)
		return
	}
	actor, _ := UserFromContext(r.Context())
	if err := h.deps.Courses.UnpublishVersion(r.Context(), actor, courseID); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "unpublished"})
}

func (h *handlers) previewVersion(w http.ResponseWriter, r *http.Request) {
	versionID, err := uuid.Parse(r.PathValue("versionId"))
	if err != nil {
		writeError(w, ErrBadRequest)
		return
	}
	actor, _ := UserFromContext(r.Context())
	v, err := h.deps.Courses.PreviewVersion(r.Context(), actor, versionID)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, v)
}

type versionMetadataRequest struct {
	Title                        string  `json:"title"`
	Summary                      string  `json:"summary"`
	DescriptionMD                string  `json:"description_md"`
	Category                     string  `json:"category"`
	Level                        string  `json:"level"`
	Language                     string  `json:"language"`
	ApprovalMinScore             float64 `json:"approval_min_score"`
	ApprovalRequiredResourcesPct float64 `json:"approval_required_resources_pct"`
}

func (h *handlers) updateVersionMetadata(w http.ResponseWriter, r *http.Request) {
	versionID, err := uuid.Parse(r.PathValue("versionId"))
	if err != nil {
		writeError(w, ErrBadRequest)
		return
	}
	var req versionMetadataRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	actor, _ := UserFromContext(r.Context())
	patch := domain.Version{
		Title: req.Title, Summary: req.Summary, DescriptionMD: req.DescriptionMD,
		Category: req.Category, Level: req.Level, Language: req.Language,
		ApprovalMinScore: req.ApprovalMinScore, ApprovalRequiredResourcesPct: req.ApprovalRequiredResourcesPct,
	}
	if err := h.deps.Courses.UpdateMetadata(r.Context(), actor, versionID, patch); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

func (h *handlers) publishVersion(w http.ResponseWriter, r *http.Request) {
	versionID, err := uuid.Parse(r.PathValue("versionId"))
	if err != nil {
		writeError(w, ErrBadRequest)
		return
	}
	actor, _ := UserFromContext(r.Context())
	err = h.deps.Courses.PublishVersion(r.Context(), actor, versionID)
	if err != nil {
		var ve *domain.ValidationError
		if errors.As(err, &ve) {
			writeError(w, asValidationError(ve.Reasons))
			return
		}
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "published"})
}

type moduleRequest struct {
	Title    string `json:"title"`
	Position int    `json:"position"`
}

func (h *handlers) addModule(w http.ResponseWriter, r *http.Request) {
	versionID, err := uuid.Parse(r.PathValue("versionId"))
	if err != nil {
		writeError(w, ErrBadRequest)
		return
	}
	var req moduleRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	actor, _ := UserFromContext(r.Context())
	m, err := h.deps.Courses.AddModule(r.Context(), actor, versionID, req.Title, req.Position)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, m)
}

func (h *handlers) updateModule(w http.ResponseWriter, r *http.Request) {
	versionID, err1 := uuid.Parse(r.PathValue("versionId"))
	moduleID, err2 := uuid.Parse(r.PathValue("moduleId"))
	if err1 != nil || err2 != nil {
		writeError(w, ErrBadRequest)
		return
	}
	var req moduleRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	actor, _ := UserFromContext(r.Context())
	if err := h.deps.Courses.UpdateModule(r.Context(), actor, versionID, moduleID, req.Title, req.Position); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

func (h *handlers) deleteModule(w http.ResponseWriter, r *http.Request) {
	versionID, err1 := uuid.Parse(r.PathValue("versionId"))
	moduleID, err2 := uuid.Parse(r.PathValue("moduleId"))
	if err1 != nil || err2 != nil {
		writeError(w, ErrBadRequest)
		return
	}
	actor, _ := UserFromContext(r.Context())
	if err := h.deps.Courses.DeleteModule(r.Context(), actor, versionID, moduleID); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusNoContent, nil)
}

type unitRequest struct {
	Title    string `json:"title"`
	Position int    `json:"position"`
}

func (h *handlers) addUnit(w http.ResponseWriter, r *http.Request) {
	versionID, err1 := uuid.Parse(r.PathValue("versionId"))
	moduleID, err2 := uuid.Parse(r.PathValue("moduleId"))
	if err1 != nil || err2 != nil {
		writeError(w, ErrBadRequest)
		return
	}
	var req unitRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	actor, _ := UserFromContext(r.Context())
	u, err := h.deps.Courses.AddUnit(r.Context(), actor, versionID, moduleID, req.Title, req.Position)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, u)
}

func (h *handlers) updateUnit(w http.ResponseWriter, r *http.Request) {
	versionID, err1 := uuid.Parse(r.PathValue("versionId"))
	unitID, err2 := uuid.Parse(r.PathValue("unitId"))
	if err1 != nil || err2 != nil {
		writeError(w, ErrBadRequest)
		return
	}
	var req unitRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	actor, _ := UserFromContext(r.Context())
	if err := h.deps.Courses.UpdateUnit(r.Context(), actor, versionID, unitID, req.Title, req.Position); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

func (h *handlers) deleteUnit(w http.ResponseWriter, r *http.Request) {
	versionID, err1 := uuid.Parse(r.PathValue("versionId"))
	unitID, err2 := uuid.Parse(r.PathValue("unitId"))
	if err1 != nil || err2 != nil {
		writeError(w, ErrBadRequest)
		return
	}
	actor, _ := UserFromContext(r.Context())
	if err := h.deps.Courses.DeleteUnit(r.Context(), actor, versionID, unitID); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusNoContent, nil)
}

type resourceRequest struct {
	Type          string `json:"type"`
	Title         string `json:"title"`
	Position      int    `json:"position"`
	Visible       bool   `json:"visible"`
	Required      bool   `json:"required"`
	Downloadable  bool   `json:"downloadable"`
	TextContentMD string `json:"text_content_md"`
	ExternalURL   string `json:"external_url"`
}

func (h *handlers) addResource(w http.ResponseWriter, r *http.Request) {
	versionID, err1 := uuid.Parse(r.PathValue("versionId"))
	unitID, err2 := uuid.Parse(r.PathValue("unitId"))
	if err1 != nil || err2 != nil {
		writeError(w, ErrBadRequest)
		return
	}
	var req resourceRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	actor, _ := UserFromContext(r.Context())
	res := &domain.Resource{
		UnitID: unitID, Type: domain.ResourceType(req.Type), Title: req.Title, Position: req.Position,
		Visible: req.Visible, Required: req.Required, Downloadable: req.Downloadable,
		TextContentMD: req.TextContentMD, ExternalURL: req.ExternalURL,
	}
	created, err := h.deps.Courses.AddResource(r.Context(), actor, versionID, res)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, created)
}

func (h *handlers) updateResource(w http.ResponseWriter, r *http.Request) {
	versionID, err1 := uuid.Parse(r.PathValue("versionId"))
	resourceID, err2 := uuid.Parse(r.PathValue("resourceId"))
	if err1 != nil || err2 != nil {
		writeError(w, ErrBadRequest)
		return
	}
	var req resourceRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	actor, _ := UserFromContext(r.Context())
	res := &domain.Resource{
		ID: resourceID, Type: domain.ResourceType(req.Type), Title: req.Title, Position: req.Position,
		Visible: req.Visible, Required: req.Required, Downloadable: req.Downloadable,
		TextContentMD: req.TextContentMD, ExternalURL: req.ExternalURL,
	}
	if err := h.deps.Courses.UpdateResource(r.Context(), actor, versionID, res); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

func (h *handlers) deleteResource(w http.ResponseWriter, r *http.Request) {
	versionID, err1 := uuid.Parse(r.PathValue("versionId"))
	resourceID, err2 := uuid.Parse(r.PathValue("resourceId"))
	if err1 != nil || err2 != nil {
		writeError(w, ErrBadRequest)
		return
	}
	actor, _ := UserFromContext(r.Context())
	if err := h.deps.Courses.DeleteResource(r.Context(), actor, versionID, resourceID); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusNoContent, nil)
}

type uploadURLRequest struct {
	MimeType string `json:"mime_type"`
}

// requestResourceUploadURL emite una URL prefirmada de subida directa al
// almacenamiento de objetos para el recurso indicado. La API nunca recibe
// el binario: el cliente sube directamente y luego confirma para encolar el
// procesamiento asíncrono si aplica.
func (h *handlers) requestResourceUploadURL(w http.ResponseWriter, r *http.Request) {
	versionID, err1 := uuid.Parse(r.PathValue("versionId"))
	resourceID, err2 := uuid.Parse(r.PathValue("resourceId"))
	if err1 != nil || err2 != nil {
		writeError(w, ErrBadRequest)
		return
	}
	var req uploadURLRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	actor, _ := UserFromContext(r.Context())
	res, err := h.deps.Courses.GetResource(r.Context(), actor, versionID, resourceID)
	if err != nil {
		writeError(w, err)
		return
	}

	objectKey := "resources/" + res.ID.String() + "/original"
	putURL, err := h.deps.Storage.PresignedPutURL(r.Context(), objectKey, 24*time.Hour)
	if err != nil {
		writeError(w, err)
		return
	}
	if err := h.deps.Courses.SetResourceObjectKey(r.Context(), actor, versionID, resourceID, objectKey, domain.ProcessingPending); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"upload_url": putURL, "object_key": objectKey, "method": "PUT"})
}

// confirmResourceUpload se llama tras completar la subida directa al
// almacenamiento. Verifica integridad (el objeto existe y tiene contenido)
// y, si el tipo de recurso requiere procesamiento asíncrono (video/audio),
// registra el activo multimedia y encola la transcodificación a HLS; en
// caso contrario marca el recurso listo de inmediato.
func (h *handlers) confirmResourceUpload(w http.ResponseWriter, r *http.Request) {
	versionID, err1 := uuid.Parse(r.PathValue("versionId"))
	resourceID, err2 := uuid.Parse(r.PathValue("resourceId"))
	if err1 != nil || err2 != nil {
		writeError(w, ErrBadRequest)
		return
	}
	actor, _ := UserFromContext(r.Context())

	res, err := h.deps.Courses.GetResource(r.Context(), actor, versionID, resourceID)
	if err != nil {
		writeError(w, err)
		return
	}
	if res.ObjectKey == "" {
		writeError(w, ErrBadRequest)
		return
	}

	info, err := h.deps.Storage.Metadatos(r.Context(), res.ObjectKey)
	if err != nil || info.Tamano == 0 {
		writeError(w, errors.New("el objeto cargado no existe o está vacío"))
		return
	}

	if !res.Type.RequiresAsyncProcessing() {
		if err := h.deps.Courses.MarkResourceProcessingStatus(r.Context(), actor, versionID, resourceID, domain.ProcessingReady); err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
		return
	}

	assetID := uuid.New()
	if err := h.deps.Media.Create(r.Context(), &postgres.MediaAsset{
		ID: assetID, ResourceID: resourceID, OriginalObjectKey: res.ObjectKey,
		MimeType: info.ContentType, SizeBytes: info.Tamano, Status: "uploaded",
	}); err != nil {
		writeError(w, err)
		return
	}

	kind := "video"
	if res.Type == domain.ResourceAudio {
		kind = "audio"
	}
	task, err := queueTask(queue.TaskProcessMedia, queue.MediaProcessPayload{
		TaskID: assetID, MediaAssetID: assetID, ResourceID: resourceID,
		SourceObjectKey: res.ObjectKey, Kind: kind,
	})
	if err != nil {
		writeError(w, err)
		return
	}
	if _, err := h.deps.Queue.Enqueue(task, asynq.MaxRetry(queue.MaxRetry), asynq.TaskID(assetID.String())); err != nil {
		writeError(w, err)
		return
	}

	if err := h.deps.Courses.MarkResourceProcessingStatus(r.Context(), actor, versionID, resourceID, domain.ProcessingPending); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"status": "queued", "media_asset_id": assetID.String()})
}

func (h *handlers) listCatalog(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit, _ := strconv.Atoi(q.Get("limit"))
	offset, _ := strconv.Atoi(q.Get("offset"))
	items, err := h.deps.Courses.ListCatalog(r.Context(), postgres.CatalogFilter{
		Search: q.Get("q"), Category: q.Get("category"), Level: q.Get("level"), Limit: limit, Offset: offset,
	})
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (h *handlers) getPublishedCourse(w http.ResponseWriter, r *http.Request) {
	courseID, err := uuid.Parse(r.PathValue("courseId"))
	if err != nil {
		writeError(w, ErrBadRequest)
		return
	}
	_, v, err := h.deps.Courses.GetPublishedByCourseID(r.Context(), courseID)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, v)
}

type initiateMultipartRequest struct {
	ContentType string `json:"content_type"`
}

func (h *handlers) initiateMultipartUpload(w http.ResponseWriter, r *http.Request) {
	versionID, err1 := uuid.Parse(r.PathValue("versionId"))
	resourceID, err2 := uuid.Parse(r.PathValue("resourceId"))
	if err1 != nil || err2 != nil {
		writeError(w, ErrBadRequest)
		return
	}
	var req initiateMultipartRequest
	_ = decodeJSON(w, r, &req)
	if req.ContentType == "" {
		req.ContentType = "application/octet-stream"
	}

	actor, _ := UserFromContext(r.Context())
	res, err := h.deps.Courses.GetResource(r.Context(), actor, versionID, resourceID)
	if err != nil {
		writeError(w, err)
		return
	}

	objectKey := "resources/" + res.ID.String() + "/original"
	uploadID, err := h.deps.Storage.InitiateMultipartUpload(r.Context(), objectKey, req.ContentType)
	if err != nil {
		writeError(w, err)
		return
	}

	if err := h.deps.Courses.SetResourceObjectKey(r.Context(), actor, versionID, resourceID, objectKey, domain.ProcessingPending); err != nil {
		writeError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"upload_id":  uploadID,
		"object_key": objectKey,
	})
}

type multipartPartURLRequest struct {
	UploadID   string `json:"upload_id"`
	PartNumber int    `json:"part_number"`
}

func (h *handlers) requestMultipartPartURL(w http.ResponseWriter, r *http.Request) {
	versionID, err1 := uuid.Parse(r.PathValue("versionId"))
	resourceID, err2 := uuid.Parse(r.PathValue("resourceId"))
	if err1 != nil || err2 != nil {
		writeError(w, ErrBadRequest)
		return
	}
	var req multipartPartURLRequest
	if !decodeJSON(w, r, &req) || req.UploadID == "" || req.PartNumber < 1 {
		writeError(w, ErrBadRequest)
		return
	}

	actor, _ := UserFromContext(r.Context())
	res, err := h.deps.Courses.GetResource(r.Context(), actor, versionID, resourceID)
	if err != nil {
		writeError(w, err)
		return
	}
	if res.ObjectKey == "" {
		writeError(w, ErrBadRequest)
		return
	}

	partURL, err := h.deps.Storage.PresignedUploadPartURL(r.Context(), res.ObjectKey, req.UploadID, req.PartNumber, 24*time.Hour)
	if err != nil {
		writeError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"upload_url":  partURL,
		"part_number": req.PartNumber,
	})
}

type completeMultipartPart struct {
	PartNumber int    `json:"part_number"`
	ETag       string `json:"etag"`
}

type completeMultipartRequest struct {
	UploadID       string                  `json:"upload_id"`
	Parts          []completeMultipartPart `json:"parts"`
	ChecksumSHA256 string                  `json:"checksum_sha256,omitempty"`
}

func (h *handlers) completeMultipartUpload(w http.ResponseWriter, r *http.Request) {
	versionID, err1 := uuid.Parse(r.PathValue("versionId"))
	resourceID, err2 := uuid.Parse(r.PathValue("resourceId"))
	if err1 != nil || err2 != nil {
		writeError(w, ErrBadRequest)
		return
	}
	var req completeMultipartRequest
	if !decodeJSON(w, r, &req) || req.UploadID == "" || len(req.Parts) == 0 {
		writeError(w, ErrBadRequest)
		return
	}

	actor, _ := UserFromContext(r.Context())
	res, err := h.deps.Courses.GetResource(r.Context(), actor, versionID, resourceID)
	if err != nil {
		writeError(w, err)
		return
	}
	if res.ObjectKey == "" {
		writeError(w, ErrBadRequest)
		return
	}

	partes := make([]storage.ParteCargada, len(req.Parts))
	for i, p := range req.Parts {
		partes[i] = storage.ParteCargada{Numero: p.PartNumber, ETag: p.ETag}
	}

	if err := h.deps.Storage.CerrarCargaMultiparte(r.Context(), res.ObjectKey, req.UploadID, partes); err != nil {
		writeError(w, err)
		return
	}

	info, err := h.deps.Storage.Metadatos(r.Context(), res.ObjectKey)
	if err != nil || info.Tamano == 0 {
		writeError(w, errors.New("el objeto cargado no existe o está vacío"))
		return
	}

	// Verificación de integridad por Checksum SHA-256 si fue provisto
	calculatedSHA256, shaErr := h.deps.Storage.CalculateSHA256(r.Context(), res.ObjectKey)
	if req.ChecksumSHA256 != "" && shaErr == nil {
		if calculatedSHA256 != req.ChecksumSHA256 {
			_ = h.deps.Storage.RemoveObject(r.Context(), res.ObjectKey)
			writeError(w, errors.New("fallo de integridad: el checksum SHA-256 no coincide"))
			return
		}
	}

	realMime, mimeErr := h.deps.Storage.DetectMIME(r.Context(), res.ObjectKey)
	detectedMime := info.ContentType
	if mimeErr == nil && realMime != "" {
		detectedMime = realMime
	}

	// Filtro de seguridad: bloquear ejecutables binarios
	if detectedMime == "application/x-dosexec" || detectedMime == "application/x-executable" || detectedMime == "application/x-sharedlib" {
		_ = h.deps.Storage.RemoveObject(r.Context(), res.ObjectKey)
		writeError(w, errors.New("el archivo contiene un binario ejecutable no permitido"))
		return
	}

	if !res.Type.RequiresAsyncProcessing() {
		if err := h.deps.Courses.MarkResourceProcessingStatus(r.Context(), actor, versionID, resourceID, domain.ProcessingReady); err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"status":          "ready",
			"mime_type":       detectedMime,
			"size_bytes":      info.Tamano,
			"checksum_sha256": calculatedSHA256,
		})
		return
	}

	assetID := uuid.New()
	if err := h.deps.Media.Create(r.Context(), &postgres.MediaAsset{
		ID: assetID, ResourceID: resourceID, OriginalObjectKey: res.ObjectKey,
		MimeType: detectedMime, SizeBytes: info.Tamano, Status: "uploaded",
	}); err != nil {
		writeError(w, err)
		return
	}

	kind := "video"
	if res.Type == domain.ResourceAudio {
		kind = "audio"
	}
	task, err := queueTask(queue.TaskProcessMedia, queue.MediaProcessPayload{
		TaskID: assetID, MediaAssetID: assetID, ResourceID: resourceID,
		SourceObjectKey: res.ObjectKey, Kind: kind,
	})
	if err != nil {
		writeError(w, err)
		return
	}
	if _, err := h.deps.Queue.Enqueue(task, asynq.MaxRetry(queue.MaxRetry), asynq.TaskID(assetID.String())); err != nil {
		writeError(w, err)
		return
	}

	if err := h.deps.Courses.MarkResourceProcessingStatus(r.Context(), actor, versionID, resourceID, domain.ProcessingPending); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{
		"status":          "queued",
		"media_asset_id":  assetID.String(),
		"size_bytes":      info.Tamano,
		"checksum_sha256": calculatedSHA256,
	})
}

func (h *handlers) listMultipartParts(w http.ResponseWriter, r *http.Request) {
	versionID, err1 := uuid.Parse(r.PathValue("versionId"))
	resourceID, err2 := uuid.Parse(r.PathValue("resourceId"))
	uploadID := r.URL.Query().Get("upload_id")
	if err1 != nil || err2 != nil || uploadID == "" {
		writeError(w, ErrBadRequest)
		return
	}

	actor, _ := UserFromContext(r.Context())
	res, err := h.deps.Courses.GetResource(r.Context(), actor, versionID, resourceID)
	if err != nil {
		writeError(w, err)
		return
	}
	if res.ObjectKey == "" {
		writeError(w, ErrBadRequest)
		return
	}

	partes, err := h.deps.Storage.PartesYaSubidas(r.Context(), res.ObjectKey, uploadID)
	if err != nil {
		writeError(w, err)
		return
	}

	// Se devuelve el número y el tamaño de cada parte ya presente: es lo que
	// el cliente necesita para saber por dónde seguir sin volver a subirlas.
	lista := make([]map[string]any, 0, len(partes))
	for _, pt := range partes {
		lista = append(lista, map[string]any{
			"part_number": pt.Numero, "size_bytes": pt.Tamano, "etag": pt.ETag,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"upload_id": uploadID,
		"parts":     lista,
	})
}
