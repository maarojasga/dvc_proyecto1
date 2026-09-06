// Package enrollment contiene las reglas de inscripción, retiro y
// reinscripción de estudiantes a cursos, conservando progreso y resultados.
package enrollment

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

// Status es el estado de una inscripción.
type Status string

const (
	StatusActive    Status = "active"
	StatusWithdrawn Status = "withdrawn"
	StatusCompleted Status = "completed"
	StatusApproved  Status = "approved"
)

var (
	ErrAlreadyEnrolled    = errors.New("enrollment: el estudiante ya está inscrito y activo en este curso")
	ErrNotEnrolled        = errors.New("enrollment: el estudiante no está inscrito en este curso")
	ErrCourseNotPublished = errors.New("enrollment: el curso no tiene una versión publicada")
)

// Enrollment vincula a un estudiante con la versión del curso que está
// cursando. course_version_id se actualiza cuando el estudiante continúa
// tras una republicación, preservando el progreso vía StableID.
type Enrollment struct {
	ID              uuid.UUID
	StudentID       uuid.UUID
	CourseID        uuid.UUID
	CourseVersionID uuid.UUID
	Status          Status
	EnrolledAt      time.Time
	WithdrawnAt     *time.Time
	CompletedAt     *time.Time
	ApprovedAt      *time.Time
}

// Withdraw retira al estudiante conservando el historial de progreso.
func (e *Enrollment) Withdraw(now time.Time) error {
	if e.Status == StatusWithdrawn {
		return nil
	}
	e.Status = StatusWithdrawn
	e.WithdrawnAt = &now
	return nil
}

// Reenroll reactiva una inscripción retirada, conservando el progreso
// previamente registrado por StableID de recurso.
func (e *Enrollment) Reenroll(now time.Time, targetVersionID uuid.UUID) {
	e.Status = StatusActive
	e.WithdrawnAt = nil
	e.CourseVersionID = targetVersionID
	e.EnrolledAt = now
}

// Complete marca la inscripción como completada (todos los recursos
// obligatorios finalizados), sin implicar aprobación de evaluaciones.
func (e *Enrollment) Complete(now time.Time) {
	if e.Status == StatusWithdrawn {
		return
	}
	e.Status = StatusCompleted
	e.CompletedAt = &now
}

// Approve marca la inscripción como aprobada, habilitando la emisión de
// insignia. Requiere que Complete se haya alcanzado antes.
func (e *Enrollment) Approve(now time.Time) {
	e.Status = StatusApproved
	e.ApprovedAt = &now
}
