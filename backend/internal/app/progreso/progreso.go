// Package progreso implementa el alcance 5.1.9: avance validado por servidor,
// finalizacion, aprobacion e insignias unicas verificables.
//
// La regla que ordena el paquete es que el porcentaje lo calcula el servidor.
// El cliente solo reporta senales (abrio, sigue ahi, cerro) y el servidor
// decide cuanto tiempo acredita comparando marcas de tiempo propias. Un
// porcentaje enviado por el cliente no se acepta: se descarta y se audita.
package progreso

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"

	"github.com/DES-SOLUCIONES-CLOUD/proyecto-1/backend/internal/domain/badge"
	"github.com/DES-SOLUCIONES-CLOUD/proyecto-1/backend/internal/domain/course"
	"github.com/DES-SOLUCIONES-CLOUD/proyecto-1/backend/internal/domain/enrollment"
	"github.com/DES-SOLUCIONES-CLOUD/proyecto-1/backend/internal/domain/progress"
	"github.com/DES-SOLUCIONES-CLOUD/proyecto-1/backend/internal/domain/user"
	"github.com/DES-SOLUCIONES-CLOUD/proyecto-1/backend/internal/platform/postgres"
)

// ErrTipoDeEventoInvalido indica una senal que no es open, heartbeat ni close.
var ErrTipoDeEventoInvalido = errors.New("progreso: tipo de evento no reconocido")

// PermanenciaMinima es el tiempo que hay que acumular sobre un recurso antes
// de darlo por completado al cerrarlo.
//
// Sin este umbral bastaria abrir y cerrar para acreditar avance, que es
// justo la manipulacion que la condicion "Progreso verificable" pide rechazar.
const PermanenciaMinima = 20 * time.Second

// Auditor deja constancia en la bitacora inmutable. Se declara como interfaz,
// satisfecha por postgres.UserRepo, para no acoplar este paquete al
// repositorio concreto y poder sustituirlo en las pruebas.
type Auditor interface {
	InsertAudit(ctx context.Context, e postgres.AuditEntry) error
}

// auditar registra un hecho sin interrumpir la operacion en curso.
//
// Si la bitacora falla, se pierde la anotacion pero no el progreso del
// estudiante: penalizar su avance por un fallo de infraestructura seria peor.
func (s *Service) auditar(ctx context.Context, actorID uuid.UUID, accion, entidad string, entidadID uuid.UUID, detalle string) {
	if s.auditor == nil {
		return
	}
	meta, err := json.Marshal(map[string]string{"detalle": detalle})
	if err != nil {
		return
	}
	_ = s.auditor.InsertAudit(ctx, postgres.AuditEntry{
		ActorUserID: &actorID, Action: accion, EntityType: entidad,
		EntityID: entidadID.String(), Metadata: meta,
	})
}

type Service struct {
	progreso    *postgres.ProgressRepo
	enrollments *postgres.EnrollmentRepo
	courses     *postgres.CourseRepo
	quizzes     *postgres.QuizRepo
	insignias   *postgres.BadgeRepo
	auditor     Auditor
	almacen     AlmacenDeInsignias
	ahora       func() time.Time
}

func NewService(
	p *postgres.ProgressRepo,
	e *postgres.EnrollmentRepo,
	c *postgres.CourseRepo,
	q *postgres.QuizRepo,
	b *postgres.BadgeRepo,
	auditor Auditor,
	almacen AlmacenDeInsignias,
) *Service {
	return &Service{
		progreso: p, enrollments: e, courses: c, quizzes: q, insignias: b,
		auditor: auditor, almacen: almacen,
		ahora: func() time.Time { return time.Now().UTC() },
	}
}

// Resumen es el avance de una inscripcion tal como se le presenta al
// estudiante.
type Resumen struct {
	EnrollmentID          uuid.UUID  `json:"enrollment_id"`
	CourseID              uuid.UUID  `json:"course_id"`
	Status                string     `json:"status"`
	PorcentajeObligatorio float64    `json:"required_percent"`
	ObligatoriosTotal     int        `json:"required_total"`
	ObligatoriosHechos    int        `json:"required_completed"`
	QuizzesPendientes     int        `json:"quizzes_pending"`
	CompletedAt           *time.Time `json:"completed_at,omitempty"`
	ApprovedAt            *time.Time `json:"approved_at,omitempty"`
	CodigoInsignia        string     `json:"badge_code,omitempty"`
}

// RegistrarEvento acredita (o descarta) una senal de progreso.
//
// El evento crudo se persiste siempre, tambien cuando se rechaza: la condicion
// verificable exige auditar la manipulacion, no solo ignorarla.
func (s *Service) RegistrarEvento(ctx context.Context, actor *user.User, resourceID uuid.UUID, tipo string, cerrar bool) (*Resumen, error) {
	tipoEvento := progress.EventType(tipo)
	switch tipoEvento {
	case progress.EventOpen, progress.EventHeartbeat, progress.EventClose:
	default:
		return nil, ErrTipoDeEventoInvalido
	}

	rec, insc, err := s.autorizar(ctx, actor, resourceID)
	if err != nil {
		return nil, err
	}

	ahora := s.ahora()
	anterior, hubo, err := s.progreso.UltimoEvento(ctx, insc.ID, rec.StableID)
	if err != nil {
		return nil, err
	}

	estado, err := s.progreso.EstadoRecurso(ctx, insc.ID, rec.StableID)
	if err != nil {
		return nil, err
	}

	acreditado := time.Duration(0)
	rechazado := false
	if tipoEvento == progress.EventHeartbeat && hubo {
		acreditado, err = progress.ValidateHeartbeat(anterior, ahora)
		if err != nil {
			// La senal llego antes de lo plausible. Se descarta el credito y
			// se dejan las dos huellas: el evento y la entrada de auditoria.
			rechazado = true
			acreditado = 0
		}
	}

	evento := &progress.Event{
		ID: uuid.New(), EnrollmentID: insc.ID, ResourceStableID: rec.StableID,
		Type: tipoEvento, DurationSeconds: int(acreditado.Seconds()), OccurredAt: ahora,
	}
	if err := s.progreso.RegistrarEvento(ctx, evento); err != nil {
		return nil, err
	}
	if rechazado {
		s.auditar(ctx, actor.ID, "progress.rejected", "resource", rec.StableID,
			"heartbeat implausible: llego antes del intervalo minimo")
	}

	// Cualquier senal implica presencia: si la primera que llega es un cierre
	// (por ejemplo tras recargar la pagina), el recurso igual quedo abierto en
	// algun momento y no tendria sentido dejarlo como no empezado.
	if estado.FirstOpenedAt == nil {
		estado.FirstOpenedAt = &ahora
	}
	if estado.Status == "not_started" {
		estado.Status = "in_progress"
	}
	if acreditado > 0 {
		estado.ApplyHeartbeat(acreditado, ahora)
	}

	// Un recurso se completa al cerrarlo tras haber permanecido lo suficiente,
	// o cuando el cliente lo declara terminado en material que no se mide por
	// tiempo (texto, imagen, enlace).
	if cerrar || tipoEvento == progress.EventClose {
		if s.puedeCompletar(rec.Type, estado) {
			estado.MarkCompleted(ahora)
		}
	}

	if err := s.progreso.GuardarEstado(ctx, estado); err != nil {
		return nil, err
	}
	return s.recalcular(ctx, insc)
}

// puedeCompletar decide si el recurso reune lo necesario para darse por visto.
//
// El material temporal (video y audio) exige permanencia acreditada; el
// material estatico se completa con abrirlo y cerrarlo, porque ahi el tiempo
// no dice nada util.
func (s *Service) puedeCompletar(tipo string, estado *progress.ResourceState) bool {
	switch tipo {
	case "video", "audio":
		return time.Duration(estado.TimeSpentSeconds)*time.Second >= PermanenciaMinima
	default:
		return estado.FirstOpenedAt != nil
	}
}

// Resumen devuelve el avance actual sin registrar ninguna senal.
func (s *Service) Resumen(ctx context.Context, actor *user.User, courseID uuid.UUID) (*Resumen, error) {
	insc, err := s.enrollments.GetByStudentAndCourse(ctx, actor.ID, courseID)
	if err != nil {
		if errors.Is(err, postgres.ErrNotFound) {
			return nil, enrollment.ErrNotEnrolled
		}
		return nil, err
	}
	return s.recalcular(ctx, insc)
}

// RecalcularCurso reevalua finalizacion y aprobacion. Lo llama el servicio de
// quizzes cuando se cierra un intento, porque aprobar una evaluacion puede ser
// lo ultimo que faltaba.
func (s *Service) RecalcularCurso(ctx context.Context, e *enrollment.Enrollment) error {
	_, err := s.recalcular(ctx, e)
	return err
}

// QuizAprobado da por completado el recurso de la evaluacion superada.
//
// Un quiz no se mide por permanencia como un video: lo que acredita haberlo
// hecho es aprobarlo. Sin esto un curso cuyo unico obligatorio fuera el quiz
// no llegaria nunca a completado, aunque el estudiante lo hubiera aprobado.
func (s *Service) QuizAprobado(ctx context.Context, e *enrollment.Enrollment, resourceStableID uuid.UUID) error {
	estado, err := s.progreso.EstadoRecurso(ctx, e.ID, resourceStableID)
	if err != nil {
		return err
	}
	ahora := s.ahora()
	if estado.FirstOpenedAt == nil {
		estado.FirstOpenedAt = &ahora
	}
	estado.MarkCompleted(ahora)
	if err := s.progreso.GuardarEstado(ctx, estado); err != nil {
		return err
	}
	_, err = s.recalcular(ctx, e)
	return err
}

// recalcular es el corazon del punto 9: mide el avance sobre los recursos
// obligatorios de la version cursada, decide finalizacion y aprobacion, y
// emite la insignia cuando corresponde.
func (s *Service) recalcular(ctx context.Context, insc *enrollment.Enrollment) (*Resumen, error) {
	version, err := s.courses.GetVersion(ctx, insc.CourseVersionID)
	if err != nil {
		return nil, err
	}
	modulos, err := s.courses.LoadTree(ctx, insc.CourseVersionID)
	if err != nil {
		return nil, err
	}

	var obligatorios []uuid.UUID
	var quizzesObligatorios []uuid.UUID
	for _, m := range modulos {
		for _, u := range m.Units {
			for _, rec := range u.Resources {
				if !rec.Visible || !rec.Required {
					continue
				}
				obligatorios = append(obligatorios, rec.StableID)
				if rec.Type == course.ResourceType("quiz") {
					quizzesObligatorios = append(quizzesObligatorios, rec.StableID)
				}
			}
		}
	}

	estados, err := s.progreso.EstadosPorInscripcion(ctx, insc.ID)
	if err != nil {
		return nil, err
	}
	porcentaje, completo := progress.CourseCompletion(obligatorios, estados, version.ApprovalRequiredResourcesPct)

	hechos := 0
	for _, id := range obligatorios {
		if st, ok := estados[id]; ok && st.Status == "completed" {
			hechos++
		}
	}

	// La aprobacion exige ademas superar todas las evaluaciones obligatorias.
	// Terminar de ver el material no basta para aprobar.
	resultados, err := s.quizzes.ResultadoPorRecurso(ctx, insc.ID)
	if err != nil {
		return nil, err
	}
	pendientes := 0
	for _, id := range quizzesObligatorios {
		if aprobado, ok := resultados[id]; !ok || !aprobado {
			pendientes++
		}
	}

	res := &Resumen{
		EnrollmentID: insc.ID, CourseID: insc.CourseID,
		PorcentajeObligatorio: porcentaje,
		ObligatoriosTotal:     len(obligatorios),
		ObligatoriosHechos:    hechos,
		QuizzesPendientes:     pendientes,
	}

	// Un curso sin ningun recurso obligatorio no acredita nada: CourseCompletion
	// devuelve 100 sobre una lista vacia, y sin esta guarda bastaria inscribirse
	// para quedar aprobado y recibir una insignia verificable sin haber hecho
	// nada. Emitir una credencial en esas condiciones seria peor que no
	// emitirla, asi que el avance se registra pero el estado no progresa.
	acreditable := len(obligatorios) > 0

	ahora := s.ahora()
	cambio := false
	if acreditable && completo && insc.Status == enrollment.StatusActive {
		insc.Complete(ahora)
		cambio = true
	}
	if acreditable && completo && pendientes == 0 && insc.Status == enrollment.StatusCompleted {
		insc.Approve(ahora)
		cambio = true
	}
	if cambio {
		if err := s.enrollments.Update(ctx, insc); err != nil {
			return nil, err
		}
	}

	if insc.Status == enrollment.StatusApproved {
		ins, err := s.emitirInsignia(ctx, insc, ahora)
		if err != nil {
			return nil, err
		}
		res.CodigoInsignia = ins.VerificationCode
	}

	res.Status = string(insc.Status)
	res.CompletedAt, res.ApprovedAt = insc.CompletedAt, insc.ApprovedAt
	return res, nil
}

// emitirInsignia crea la insignia una sola vez.
//
// La idempotencia la garantiza la restriccion unica sobre enrollment_id: dos
// llamadas concurrentes terminan devolviendo la misma insignia en vez de
// crear dos.
func (s *Service) emitirInsignia(ctx context.Context, insc *enrollment.Enrollment, ahora time.Time) (*badge.Badge, error) {
	if existente, err := s.insignias.PorInscripcion(ctx, insc.ID); err == nil {
		return existente, nil
	} else if !errors.Is(err, postgres.ErrNotFound) {
		return nil, err
	}

	nueva, err := badge.New(insc.ID, insc.CourseID, insc.StudentID, ahora)
	if err != nil {
		return nil, err
	}
	nueva.ImageObjectKey = claveDeImagen(nueva.VerificationCode)
	emitida, err := s.insignias.Emitir(ctx, nueva)
	if err != nil {
		return nil, err
	}
	s.generarImagen(ctx, emitida)
	return emitida, nil
}

// generarImagen dibuja y guarda la imagen de la insignia.
//
// Un fallo aqui no deshace la emision: la insignia es el logro academico y la
// imagen es su representacion. Perder el dibujo por un almacen caido y con
// ello quitarle al estudiante el curso aprobado seria desproporcionado. Queda
// anotado en la bitacora y la imagen se puede regenerar.
func (s *Service) generarImagen(ctx context.Context, b *badge.Badge) {
	if s.almacen == nil || b.ImageObjectKey == "" {
		return
	}
	titulo := s.tituloDelCurso(ctx, b.CourseID)
	svg := svgDeInsignia(titulo, b.VerificationCode, b.IssuedAt)
	if err := s.almacen.SubirBytes(ctx, b.ImageObjectKey, svg, "image/svg+xml"); err != nil {
		s.auditar(ctx, b.StudentID, "badge.image_failed", "badge", b.ID, err.Error())
	}
}

// tituloDelCurso busca el titulo de la version publicada para ponerlo en la
// imagen. Si no se puede, la insignia sale con un titulo genérico antes que no
// salir.
func (s *Service) tituloDelCurso(ctx context.Context, courseID uuid.UUID) string {
	c, err := s.courses.GetCourse(ctx, courseID)
	if err != nil || c.CurrentPublishedVersionID == nil {
		return "Curso"
	}
	v, err := s.courses.GetVersion(ctx, *c.CurrentPublishedVersionID)
	if err != nil || v.Title == "" {
		return "Curso"
	}
	return v.Title
}

// URLDeImagen resuelve la URL de la imagen de una insignia.
func (s *Service) URLDeImagen(ctx context.Context, b *badge.Badge) string {
	if s.almacen == nil || b.ImageObjectKey == "" {
		return ""
	}
	url, err := s.almacen.PresignedGetURL(ctx, b.ImageObjectKey, vigenciaImagen, "insignia.svg")
	if err != nil {
		return ""
	}
	return url
}

// Verificacion es la vista publica de una insignia.
//
// No lleva el correo ni el nombre del estudiante: la condicion "Emision de
// insignia" pide que la URL publica no exponga datos personales.
type Verificacion struct {
	Code     string    `json:"code"`
	CourseID uuid.UUID `json:"course_id"`
	// CourseTitle va porque una verificacion que no dice de que curso es no
	// verifica nada util. El titulo del curso es publico: esta en el catalogo.
	CourseTitle string     `json:"course_title"`
	Valid       bool       `json:"valid"`
	IssuedAt    time.Time  `json:"issued_at"`
	RevokedAt   *time.Time `json:"revoked_at,omitempty"`
	ImageURL    string     `json:"image_url,omitempty"`
}

// Verificar resuelve la URL publica de verificacion. No exige autenticacion,
// por eso devuelve lo minimo indispensable.
func (s *Service) Verificar(ctx context.Context, codigo string) (*Verificacion, error) {
	b, err := s.insignias.PorCodigo(ctx, codigo)
	if err != nil {
		return nil, err
	}
	return &Verificacion{
		Code: b.VerificationCode, CourseID: b.CourseID,
		CourseTitle: s.tituloDelCurso(ctx, b.CourseID),
		Valid:       b.Verified(), IssuedAt: b.IssuedAt, RevokedAt: b.RevokedAt,
		ImageURL: s.URLDeImagen(ctx, b),
	}, nil
}

// InsigniaPropia es una insignia tal como se le presenta a su dueño: con el
// título del curso y la URL de la imagen ya resueltos, que es lo que la
// interfaz necesita y el modelo no guarda.
type InsigniaPropia struct {
	Code          string     `json:"code"`
	CourseID      uuid.UUID  `json:"course_id"`
	CourseTitle   string     `json:"course_title"`
	Valid         bool       `json:"valid"`
	IssuedAt      time.Time  `json:"issued_at"`
	RevokedAt     *time.Time `json:"revoked_at,omitempty"`
	RevokedReason string     `json:"revoked_reason,omitempty"`
	ImageURL      string     `json:"image_url,omitempty"`
}

// MisInsignias lista las insignias del estudiante autenticado.
func (s *Service) MisInsignias(ctx context.Context, actor *user.User) ([]InsigniaPropia, error) {
	lista, err := s.insignias.ListarPorEstudiante(ctx, actor.ID)
	if err != nil {
		return nil, err
	}
	salida := make([]InsigniaPropia, 0, len(lista))
	for _, b := range lista {
		salida = append(salida, InsigniaPropia{
			Code: b.VerificationCode, CourseID: b.CourseID,
			CourseTitle: s.tituloDelCurso(ctx, b.CourseID),
			Valid:       b.Verified(), IssuedAt: b.IssuedAt,
			RevokedAt: b.RevokedAt, RevokedReason: b.RevokedReason,
			ImageURL: s.URLDeImagen(ctx, b),
		})
	}
	return salida, nil
}

// Revocar invalida una insignia. Solo la administracion puede hacerlo y queda
// auditado, porque revocar un logro academico es una accion sensible.
func (s *Service) Revocar(ctx context.Context, actor *user.User, codigo, motivo string) error {
	if actor.Role != user.RoleAdmin {
		return postgres.ErrNotFound
	}
	b, err := s.insignias.PorCodigo(ctx, codigo)
	if err != nil {
		return err
	}
	if !b.Verified() {
		return nil
	}
	b.Revoke(motivo, s.ahora())
	if err := s.insignias.Revocar(ctx, b); err != nil {
		return err
	}
	s.auditar(ctx, actor.ID, "badge.revoked", "badge", b.ID, motivo)
	return nil
}

func (s *Service) autorizar(ctx context.Context, actor *user.User, resourceID uuid.UUID) (*postgres.RecursoPublicadoConMedia, *enrollment.Enrollment, error) {
	rec, err := s.courses.GetRecursoPublicadoConMedia(ctx, resourceID)
	if err != nil {
		return nil, nil, err
	}
	if !rec.Visible {
		return nil, nil, postgres.ErrNotFound
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
