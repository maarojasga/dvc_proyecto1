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

// ErrMediaNoLista indica que el recurso existe pero su transcodificación aún
// no ha terminado o falló.
var ErrMediaNoLista = errors.New("enrollments: el recurso multimedia todavía no está listo")

// ClaveDeReproduccion autoriza la reproducción de un recurso multimedia y
// devuelve la clave de su lista maestra HLS.
//
// Se exige que el recurso pertenezca a la versión vigente del curso, que sea
// visible y que quien lo pide esté inscrito; el profesor dueño y la
// administración pasan sin inscripción, porque necesitan revisarlo. Un
// recurso oculto o de un borrador responde como inexistente, para no delatar
// contenido que no está publicado.
func (s *Service) ClaveDeReproduccion(ctx context.Context, actor *user.User, resourceID uuid.UUID) (string, error) {
	rec, err := s.courses.GetRecursoPublicadoConMedia(ctx, resourceID)
	if err != nil {
		return "", err
	}
	if !rec.Visible {
		return "", postgres.ErrNotFound
	}

	if actor.Role != user.RoleAdmin && actor.ID != rec.TeacherID {
		insc, err := s.enrollments.GetByStudentAndCourse(ctx, actor.ID, rec.CourseID)
		if err != nil {
			if errors.Is(err, postgres.ErrNotFound) {
				return "", enrollment.ErrNotEnrolled
			}
			return "", err
		}
		if !insc.DaAccesoAlContenido() {
			return "", enrollment.ErrNotEnrolled
		}
	}

	if rec.AssetStatus != "ready" || rec.HLSMasterKey == "" {
		return "", ErrMediaNoLista
	}
	return rec.HLSMasterKey, nil
}

func (s *Service) ListMine(ctx context.Context, student *user.User) ([]*enrollment.Enrollment, error) {
	return s.enrollments.ListByStudent(ctx, student.ID)
}
