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
	progreso    *postgres.ProgressRepo
}

func NewService(enrollments *postgres.EnrollmentRepo, courses *postgres.CourseRepo, progreso *postgres.ProgressRepo) *Service {
	return &Service{enrollments: enrollments, courses: courses, progreso: progreso}
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

// ErrPosicionInvalida indica una posición fuera de rango o un tipo de recurso
// que no se reproduce.
var ErrPosicionInvalida = errors.New("enrollments: posición de reproducción inválida")

// ClaveDeReproduccion autoriza la reproducción de un recurso multimedia y
// devuelve la clave de su lista maestra HLS.
//
// Se exige que el recurso pertenezca a la versión vigente del curso, que sea
// visible y que quien lo pide esté inscrito; el profesor dueño y la
// administración pasan sin inscripción, porque necesitan revisarlo. Un
// recurso oculto o de un borrador responde como inexistente, para no delatar
// contenido que no está publicado.
// Contenido es lo necesario para presentar un recurso al estudiante.
//
// Solo uno de los campos de origen viene relleno, según el tipo: la clave del
// objeto para lo que se sirve desde el almacenamiento, el Markdown para el
// texto, o la URL externa para enlaces e iframes.
type Contenido struct {
	Tipo        string
	Titulo      string
	Descargable bool
	ClaveObjeto string
	Markdown    string
	URLExterna  string
	// Segundos donde reanudar. Solo tiene sentido en video y audio.
	PosicionSegundos int
}

// autorizarRecurso comprueba el derecho de acceso a un recurso y, de paso,
// devuelve la inscripción cuando quien pide es un estudiante.
//
// Se exige que el recurso pertenezca a la versión vigente del curso y que sea
// visible; el profesor dueño y la administración pasan sin inscripción,
// porque necesitan revisarlo. Un recurso oculto o de un borrador responde
// como inexistente, para no delatar contenido que no está publicado.
func (s *Service) autorizarRecurso(ctx context.Context, actor *user.User, resourceID uuid.UUID) (*postgres.RecursoPublicadoConMedia, *enrollment.Enrollment, error) {
	rec, err := s.courses.GetRecursoPublicadoConMedia(ctx, resourceID)
	if err != nil {
		return nil, nil, err
	}
	if !rec.Visible {
		return nil, nil, postgres.ErrNotFound
	}
	if actor.Role == user.RoleAdmin || actor.ID == rec.TeacherID {
		return rec, nil, nil
	}

	insc, err := s.enrollments.GetByStudentAndCourse(ctx, actor.ID, rec.CourseID)
	if err != nil {
		if errors.Is(err, postgres.ErrNotFound) {
			return nil, nil, enrollment.ErrNotEnrolled
		}
		return nil, nil, err
	}
	if !insc.DaAccesoAlContenido() {
		return nil, nil, enrollment.ErrNotEnrolled
	}
	return rec, insc, nil
}

// ContenidoDeRecurso autoriza y describe cómo presentar un recurso.
//
// Para video y audio devuelve la lista maestra HLS y la última posición
// reportada; para PDF y descargables, la clave del objeto; para texto, el
// Markdown; y para enlaces e iframes, la URL externa.
func (s *Service) ContenidoDeRecurso(ctx context.Context, actor *user.User, resourceID uuid.UUID) (*Contenido, error) {
	rec, insc, err := s.autorizarRecurso(ctx, actor, resourceID)
	if err != nil {
		return nil, err
	}

	out := &Contenido{
		Tipo: rec.Type, Titulo: rec.Title, Descargable: rec.Downloadable,
		Markdown: rec.TextContent, URLExterna: rec.ExternalURL,
	}

	switch rec.Type {
	case "video", "audio":
		if rec.AssetStatus != "ready" || rec.HLSMasterKey == "" {
			return nil, ErrMediaNoLista
		}
		out.ClaveObjeto = rec.HLSMasterKey
		if insc != nil && s.progreso != nil {
			// La posición es por inscripción: cada estudiante reanuda donde
			// dejó, y solo si está inscrito.
			pos, err := s.progreso.Posicion(ctx, insc.ID, rec.StableID)
			if err != nil {
				return nil, err
			}
			out.PosicionSegundos = pos
		}
	case "pdf", "image", "file", "presentation":
		if rec.ObjectKey == "" {
			return nil, ErrMediaNoLista
		}
		out.ClaveObjeto = rec.ObjectKey
	}
	return out, nil
}

// PosicionMaxima acota lo que un cliente puede reportar. Sirve para reanudar,
// no para acreditar nada, así que basta con un techo generoso que descarte
// valores absurdos.
const PosicionMaxima = 24 * 60 * 60

// GuardarPosicion registra dónde reanudar la reproducción de un recurso.
//
// Es una comodidad reportada por el cliente y no acredita avance: el progreso
// lo calcula el servidor por su cuenta. Por eso se acepta sin más validación
// que el rango, y por eso no toca el estado del recurso.
func (s *Service) GuardarPosicion(ctx context.Context, actor *user.User, resourceID uuid.UUID, segundos int) error {
	if segundos < 0 || segundos > PosicionMaxima {
		return ErrPosicionInvalida
	}
	rec, insc, err := s.autorizarRecurso(ctx, actor, resourceID)
	if err != nil {
		return err
	}
	if insc == nil {
		// Profesores y administración pueden revisar el material, pero no
		// tienen inscripción donde anotar una posición.
		return nil
	}
	if rec.Type != "video" && rec.Type != "audio" {
		return ErrPosicionInvalida
	}
	return s.progreso.GuardarPosicion(ctx, insc.ID, rec.StableID, segundos)
}

func (s *Service) ListMine(ctx context.Context, student *user.User) ([]*enrollment.Enrollment, error) {
	return s.enrollments.ListByStudent(ctx, student.ID)
}
