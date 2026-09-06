// Package enrollments implementa inscripción, retiro y reinscripción de
// estudiantes, conservando progreso y resultados entre versiones del curso.
package enrollments

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"

	"github.com/DES-SOLUCIONES-CLOUD/proyecto-1/backend/internal/domain/enrollment"
	"github.com/DES-SOLUCIONES-CLOUD/proyecto-1/backend/internal/domain/user"
	"github.com/DES-SOLUCIONES-CLOUD/proyecto-1/backend/internal/platform/postgres"
)

type Service struct {
	enrollments *postgres.EnrollmentRepo
	courses     *postgres.CourseRepo
}

func NewService(enrollments *postgres.EnrollmentRepo, courses *postgres.CourseRepo) *Service {
	return &Service{enrollments: enrollments, courses: courses}
}

// Enroll inscribe al estudiante en la versión publicada vigente del curso.
// Si ya existía una inscripción retirada, la reactiva conservando el
// historial de progreso (reinscripción).
func (s *Service) Enroll(ctx context.Context, student *user.User, courseID uuid.UUID) (*enrollment.Enrollment, error) {
	c, err := s.courses.GetCourse(ctx, courseID)
	if err != nil {
		return nil, err
	}
	if c.CurrentPublishedVersionID == nil {
		return nil, enrollment.ErrCourseNotPublished
	}

	existing, err := s.enrollments.GetByStudentAndCourse(ctx, student.ID, courseID)
	if err == nil {
		if existing.Status == enrollment.StatusActive {
			return nil, enrollment.ErrAlreadyEnrolled
		}
		existing.Reenroll(time.Now().UTC(), *c.CurrentPublishedVersionID)
		if err := s.enrollments.Update(ctx, existing); err != nil {
			return nil, err
		}
		return existing, nil
	}
	if !errors.Is(err, postgres.ErrNotFound) {
		return nil, err
	}

	e := &enrollment.Enrollment{
		ID:              uuid.New(),
		StudentID:       student.ID,
		CourseID:        courseID,
		CourseVersionID: *c.CurrentPublishedVersionID,
		Status:          enrollment.StatusActive,
		EnrolledAt:      time.Now().UTC(),
	}
	if err := s.enrollments.Create(ctx, e); err != nil {
		return nil, err
	}
	return e, nil
}

func (s *Service) Withdraw(ctx context.Context, student *user.User, courseID uuid.UUID) error {
	e, err := s.enrollments.GetByStudentAndCourse(ctx, student.ID, courseID)
	if err != nil {
		if errors.Is(err, postgres.ErrNotFound) {
			return enrollment.ErrNotEnrolled
		}
		return err
	}
	if err := e.Withdraw(time.Now().UTC()); err != nil {
		return err
	}
	return s.enrollments.Update(ctx, e)
}

func (s *Service) ListMine(ctx context.Context, student *user.User) ([]*enrollment.Enrollment, error) {
	return s.enrollments.ListByStudent(ctx, student.ID)
}
