package course

import (
	"time"

	"github.com/google/uuid"
)

// Publish transiciona una versión en borrador a publicada. El llamador debe
// haber ejecutado antes ValidateForPublish() y persistir, dentro de la misma
// transacción, que cualquier versión previamente publicada del curso pase a
// unpublished y que Course.CurrentPublishedVersionID apunte a esta versión.
func (v *Version) Publish(now time.Time) error {
	if v.Status != VersionDraft {
		return ErrNotDraft
	}
	v.Status = VersionPublished
	v.PublishedAt = &now
	v.UpdatedAt = now
	return nil
}

// Unpublish retira temporalmente una versión publicada para permitir su
// edición, tal como exige la sección 5.1 durante el MVP.
func (v *Version) Unpublish(now time.Time) error {
	if v.Status != VersionPublished {
		return ErrAlreadyPublished
	}
	v.Status = VersionUnpublished
	v.UpdatedAt = now
	return nil
}

// NewDraft crea la siguiente versión en borrador a partir del número de
// versión anterior. El contenido (módulos/unidades/recursos) se copia por el
// servicio de aplicación reasignando los mismos StableID para preservar el
// progreso de los estudiantes ya inscritos.
func NewDraft(courseID uuid.UUID, previousVersionNumber int, now time.Time) *Version {
	return &Version{
		ID:                           uuid.New(),
		CourseID:                     courseID,
		VersionNumber:                previousVersionNumber + 1,
		Status:                       VersionDraft,
		Language:                     "es",
		ApprovalMinScore:             60,
		ApprovalRequiredResourcesPct: 100,
		CreatedAt:                    now,
		UpdatedAt:                    now,
	}
}
