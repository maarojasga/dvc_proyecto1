// Package course contiene la jerarquía académica Curso -> Módulo -> Unidad ->
// Recurso, sus versiones y las reglas de publicación.
package course

import (
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// VersionStatus es el estado de una versión de curso.
type VersionStatus string

const (
	VersionDraft       VersionStatus = "draft"
	VersionPublished   VersionStatus = "published"
	VersionUnpublished VersionStatus = "unpublished"
	VersionArchived    VersionStatus = "archived"
)

// ResourceType enumera los tipos de recurso admitidos por la especificación.
type ResourceType string

const (
	ResourceText         ResourceType = "text"
	ResourceImage        ResourceType = "image"
	ResourceVideo        ResourceType = "video"
	ResourceAudio        ResourceType = "audio"
	ResourcePDF          ResourceType = "pdf"
	ResourcePresentation ResourceType = "presentation"
	ResourceFile         ResourceType = "file"
	ResourceIframe       ResourceType = "iframe"
	ResourceLink         ResourceType = "link"
	ResourceQuiz         ResourceType = "quiz"
)

func (t ResourceType) Valid() bool {
	switch t {
	case ResourceText, ResourceImage, ResourceVideo, ResourceAudio, ResourcePDF,
		ResourcePresentation, ResourceFile, ResourceIframe, ResourceLink, ResourceQuiz:
		return true
	}
	return false
}

// requiresProcessing indica si el tipo de recurso pasa por el pipeline
// asíncrono de procesamiento (transcodificación HLS). La conversión de
// presentaciones (PPTX/ODP a PDF) es alcance opcional (sección 5.2) y no
// está implementada en esta iteración: dichos recursos se marcan listos de
// inmediato tras la carga.
func (t ResourceType) requiresProcessing() bool {
	switch t {
	case ResourceVideo, ResourceAudio:
		return true
	}
	return false
}

// RequiresAsyncProcessing expone la regla a la capa de aplicación.
func (t ResourceType) RequiresAsyncProcessing() bool { return t.requiresProcessing() }

// ProcessingStatus es el estado de procesamiento asíncrono de un recurso.
type ProcessingStatus string

const (
	ProcessingNone       ProcessingStatus = "none"
	ProcessingPending    ProcessingStatus = "pending"
	ProcessingProcessing ProcessingStatus = "processing"
	ProcessingReady      ProcessingStatus = "ready"
	ProcessingFailed     ProcessingStatus = "failed"
)

// Course es el agregado raíz de autoría.
type Course struct {
	ID                        uuid.UUID
	TeacherID                 uuid.UUID
	Slug                      string
	CurrentPublishedVersionID *uuid.UUID
	CreatedAt                 time.Time
	UpdatedAt                 time.Time
}

// Version es una versión numerada del curso; solo una puede estar publicada
// a la vez para un mismo curso.
type Version struct {
	ID                           uuid.UUID
	CourseID                     uuid.UUID
	VersionNumber                int
	Status                       VersionStatus
	Title                        string
	Summary                      string
	DescriptionMD                string
	Category                     string
	Level                        string
	Language                     string
	ApprovalMinScore             float64
	ApprovalRequiredResourcesPct float64
	PublishedAt                  *time.Time
	CreatedAt                    time.Time
	UpdatedAt                    time.Time

	Modules []Module // opcional: cargado por el repositorio cuando se necesita el árbol completo
}

// Module agrupa unidades ordenadas dentro de una versión.
type Module struct {
	ID              uuid.UUID
	CourseVersionID uuid.UUID
	StableID        uuid.UUID
	Title           string
	Position        int
	Units           []Unit
}

// Unit agrupa recursos ordenados dentro de un módulo.
type Unit struct {
	ID        uuid.UUID
	ModuleID  uuid.UUID
	StableID  uuid.UUID
	Title     string
	Position  int
	Resources []Resource
}

// Resource es la unidad mínima de contenido consumible.
type Resource struct {
	ID               uuid.UUID
	UnitID           uuid.UUID
	StableID         uuid.UUID
	Type             ResourceType
	Title            string
	Position         int
	Visible          bool
	Required         bool
	Downloadable     bool
	ProcessingStatus ProcessingStatus
	TextContentMD    string
	ExternalURL      string
	ObjectKey        string
}

// IsPublishable evalúa si el recurso está listo para formar parte de una
// versión publicada: visible, y si requiere procesamiento asíncrono, que
// éste haya terminado con éxito.
func (r *Resource) IsPublishable() bool {
	if !r.Visible {
		return true // los recursos ocultos no bloquean la publicación
	}
	if r.Type.requiresProcessing() {
		return r.ProcessingStatus == ProcessingReady
	}
	return true
}

var (
	ErrTitleRequired           = errors.New("course: el título es obligatorio")
	ErrSummaryRequired         = errors.New("course: el resumen es obligatorio para publicar")
	ErrNoModules               = errors.New("course: la versión debe tener al menos un módulo")
	ErrModuleWithoutUnits      = errors.New("course: todo módulo debe tener al menos una unidad")
	ErrUnitWithoutResources    = errors.New("course: toda unidad debe tener al menos un recurso visible")
	ErrResourceNotReady        = errors.New("course: hay recursos visibles que aún no terminan de procesarse")
	ErrAlreadyPublished        = errors.New("course: la versión ya está publicada")
	ErrNotDraft                = errors.New("course: solo un borrador puede publicarse")
	ErrInvalidApprovalCriteria = errors.New("course: los criterios de aprobación deben estar entre 0 y 100")
)

// ValidationError agrega una lista de motivos de rechazo para exponer al
// autor una lista exhaustiva de errores, tal como exige la especificación.
type ValidationError struct {
	Reasons []error
}

func (e *ValidationError) Error() string {
	if len(e.Reasons) == 0 {
		return "course: versión inválida"
	}
	msg := "course: versión inválida: "
	for i, r := range e.Reasons {
		if i > 0 {
			msg += "; "
		}
		msg += r.Error()
	}
	return msg
}

// ValidateForPublish aplica todas las condiciones de "publicación válida"
// de la sección 6 de la especificación y devuelve la lista exhaustiva de
// motivos de rechazo (no solo el primero).
func (v *Version) ValidateForPublish() error {
	var reasons []error

	if v.Status != VersionDraft {
		reasons = append(reasons, ErrNotDraft)
	}
	if v.Title == "" {
		reasons = append(reasons, ErrTitleRequired)
	}
	if v.Summary == "" {
		reasons = append(reasons, ErrSummaryRequired)
	}
	if v.ApprovalMinScore < 0 || v.ApprovalMinScore > 100 ||
		v.ApprovalRequiredResourcesPct < 0 || v.ApprovalRequiredResourcesPct > 100 {
		reasons = append(reasons, ErrInvalidApprovalCriteria)
	}
	if len(v.Modules) == 0 {
		reasons = append(reasons, ErrNoModules)
	}

	for _, m := range v.Modules {
		if len(m.Units) == 0 {
			reasons = append(reasons, wrapf(ErrModuleWithoutUnits, "módulo %q", m.Title))
			continue
		}
		for _, u := range m.Units {
			visibleCount := 0
			for _, r := range u.Resources {
				if r.Visible {
					visibleCount++
				}
				if !r.IsPublishable() {
					reasons = append(reasons, wrapf(ErrResourceNotReady, "recurso %q (unidad %q)", r.Title, u.Title))
				}
			}
			if visibleCount == 0 {
				reasons = append(reasons, wrapf(ErrUnitWithoutResources, "unidad %q (módulo %q)", u.Title, m.Title))
			}
		}
	}

	if len(reasons) > 0 {
		return &ValidationError{Reasons: reasons}
	}
	return nil
}

func wrapf(base error, format string, args ...any) error {
	return fmt.Errorf("%w: %s", base, fmt.Sprintf(format, args...))
}
