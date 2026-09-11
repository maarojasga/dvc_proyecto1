// Package quizzes implementa el alcance 5.1.8: evaluaciones de seleccion
// multiple con intentos, guardado parcial, calificacion en servidor y
// retroalimentacion configurable.
//
// La regla que ordena todo el paquete es que la clave correcta nunca sale del
// servidor. El cliente recibe preguntas y opciones sin marcar, envia lo que
// eligio, y es el servidor quien compara contra el snapshot congelado al
// iniciar el intento.
package quizzes

import (
	"context"
	"errors"
	"math/rand"
	"time"

	"github.com/google/uuid"

	"github.com/DES-SOLUCIONES-CLOUD/proyecto-1/backend/internal/app/courses"
	"github.com/DES-SOLUCIONES-CLOUD/proyecto-1/backend/internal/domain/enrollment"
	"github.com/DES-SOLUCIONES-CLOUD/proyecto-1/backend/internal/domain/quiz"
	"github.com/DES-SOLUCIONES-CLOUD/proyecto-1/backend/internal/domain/user"
	"github.com/DES-SOLUCIONES-CLOUD/proyecto-1/backend/internal/platform/postgres"
)

var (
	// ErrIntentoAjeno protege un intento de ser leido o modificado por quien
	// no lo abrio.
	ErrIntentoAjeno = errors.New("quizzes: el intento pertenece a otra inscripcion")
	// ErrRespuestaInvalida indica una pregunta u opcion que no pertenece al
	// snapshot del intento.
	ErrRespuestaInvalida = errors.New("quizzes: la respuesta no corresponde al intento")
	// ErrRecursoNoEsQuiz se produce al definir una evaluacion sobre un recurso
	// de otro tipo.
	ErrRecursoNoEsQuiz = errors.New("quizzes: el recurso no es de tipo quiz")
	// ErrDefinicionInvalida agrupa las evaluaciones que no se podrian calificar.
	ErrDefinicionInvalida = errors.New("quizzes: la definicion del quiz es invalida")
)

// Recalculador es el gancho hacia el progreso: al cerrarse un intento hay que
// reevaluar si el curso quedo aprobado. Se declara como interfaz para que este
// paquete no dependa del de progreso y no haya ciclo de importacion.
type Recalculador interface {
	RecalcularCurso(ctx context.Context, e *enrollment.Enrollment) error
	// QuizAprobado marca como completado el recurso de la evaluacion superada:
	// en un quiz lo que acredita haberlo hecho es aprobarlo, no el tiempo.
	QuizAprobado(ctx context.Context, e *enrollment.Enrollment, resourceStableID uuid.UUID) error
}

type Service struct {
	quizzes     *postgres.QuizRepo
	courseRepo  *postgres.CourseRepo
	courses     *courses.Service
	enrollments *postgres.EnrollmentRepo
	progreso    Recalculador
	ahora       func() time.Time
}

func NewService(q *postgres.QuizRepo, cr *postgres.CourseRepo, cs *courses.Service, e *postgres.EnrollmentRepo, p Recalculador) *Service {
	return &Service{
		quizzes: q, courseRepo: cr, courses: cs, enrollments: e, progreso: p,
		ahora: func() time.Time { return time.Now().UTC() },
	}
}

// VistaIntento es la proyeccion que si puede viajar al cliente: preguntas y
// opciones en el orden congelado, sin ninguna marca de cual es la correcta.
type VistaIntento struct {
	AttemptID     uuid.UUID           `json:"attempt_id"`
	QuizID        uuid.UUID           `json:"quiz_id"`
	Title         string              `json:"title"`
	AttemptNumber int                 `json:"attempt_number"`
	Status        string              `json:"status"`
	StartedAt     time.Time           `json:"started_at"`
	ExpiresAt     *time.Time          `json:"expires_at,omitempty"`
	Questions     []PreguntaVisible   `json:"questions"`
	Answers       map[string][]string `json:"answers"`
	// Score y Passed solo se rellenan cuando la politica de retroalimentacion
	// permite revelarlos.
	Score  *float64 `json:"score,omitempty"`
	Passed *bool    `json:"passed,omitempty"`
}

type PreguntaVisible struct {
	StableID uuid.UUID       `json:"stable_id"`
	PromptMD string          `json:"prompt_md"`
	Type     string          `json:"type"`
	Points   float64         `json:"points"`
	Options  []OpcionVisible `json:"options"`
}

type OpcionVisible struct {
	StableID uuid.UUID `json:"stable_id"`
	TextMD   string    `json:"text_md"`
}

// DefinicionQuiz es lo que envia el profesor al crear o editar la evaluacion.
type DefinicionQuiz struct {
	Title            string
	TimeLimitSeconds *int
	MaxAttempts      *int
	PassScore        float64
	FeedbackPolicy   string
	ShuffleQuestions bool
	Questions        []DefinicionPregunta
}

type DefinicionPregunta struct {
	PromptMD string
	Type     string
	Points   float64
	Options  []DefinicionOpcion
}

type DefinicionOpcion struct {
	TextMD    string
	IsCorrect bool
}

// Definir crea o reemplaza el quiz de un recurso.
//
// Solo el profesor dueno y sobre una version en borrador: una version
// publicada es inmutable, asi que cambiar una evaluacion exige despublicar.
func (s *Service) Definir(ctx context.Context, actor *user.User, versionID, resourceID uuid.UUID, def DefinicionQuiz) (*quiz.Quiz, error) {
	if _, _, err := s.courses.GetOwnedVersion(ctx, actor, versionID); err != nil {
		return nil, err
	}
	// La comprobacion de que el recurso pertenece a la version evita colgar un
	// quiz de un curso ajeno pasando un identificador de otro.
	rec, err := s.courseRepo.GetResource(ctx, versionID, resourceID)
	if err != nil {
		return nil, err
	}
	if string(rec.Type) != "quiz" {
		return nil, ErrRecursoNoEsQuiz
	}
	if err := validarDefinicion(def); err != nil {
		return nil, err
	}

	q := &quiz.Quiz{
		ResourceID: resourceID, Title: def.Title,
		TimeLimitSeconds: def.TimeLimitSeconds, MaxAttempts: def.MaxAttempts,
		PassScore: def.PassScore, FeedbackPolicy: quiz.FeedbackPolicy(def.FeedbackPolicy),
		ShuffleQuestions: def.ShuffleQuestions,
	}
	for i, p := range def.Questions {
		pregunta := quiz.Question{
			Position: i + 1, PromptMD: p.PromptMD,
			Type: quiz.QuestionType(p.Type), Points: p.Points,
		}
		for j, o := range p.Options {
			pregunta.Options = append(pregunta.Options, quiz.Option{
				Position: j + 1, TextMD: o.TextMD, IsCorrect: o.IsCorrect,
			})
		}
		q.Questions = append(q.Questions, pregunta)
	}
	if err := s.quizzes.GuardarDefinicion(ctx, q); err != nil {
		return nil, err
	}
	return q, nil
}

// validarDefinicion rechaza evaluaciones que no se podrian calificar.
//
// Una pregunta sin respuesta correcta seria imposible de aprobar, y una de
// respuesta unica con varias correctas es ambigua: mejor detectarlo al
// guardar que al calificar.
func validarDefinicion(def DefinicionQuiz) error {
	if def.Title == "" {
		return ErrDefinicionInvalida
	}
	switch quiz.FeedbackPolicy(def.FeedbackPolicy) {
	case quiz.FeedbackImmediate, quiz.FeedbackAfterSubmit, quiz.FeedbackAfterClose, quiz.FeedbackNone:
	default:
		return ErrDefinicionInvalida
	}
	if def.PassScore < 0 || def.PassScore > 100 {
		return ErrDefinicionInvalida
	}
	if len(def.Questions) == 0 {
		return ErrDefinicionInvalida
	}
	for _, p := range def.Questions {
		if p.PromptMD == "" || len(p.Options) < 2 || p.Points <= 0 {
			return ErrDefinicionInvalida
		}
		correctas := 0
		for _, o := range p.Options {
			if o.TextMD == "" {
				return ErrDefinicionInvalida
			}
			if o.IsCorrect {
				correctas++
			}
		}
		if correctas == 0 {
			return ErrDefinicionInvalida
		}
		if quiz.QuestionType(p.Type) == quiz.QuestionSingle && correctas != 1 {
			return ErrDefinicionInvalida
		}
		if quiz.QuestionType(p.Type) != quiz.QuestionSingle && quiz.QuestionType(p.Type) != quiz.QuestionMultiple {
			return ErrDefinicionInvalida
		}
	}
	return nil
}

// Iniciar abre un intento, o devuelve el que ya estuviera en curso.
//
// Reanudar en vez de crear otro es deliberado: recargar la pagina no debe
// gastar un intento ni perder lo guardado.
func (s *Service) Iniciar(ctx context.Context, actor *user.User, resourceID uuid.UUID) (*VistaIntento, error) {
	q, insc, err := s.autorizar(ctx, actor, resourceID)
	if err != nil {
		return nil, err
	}

	if abierto, err := s.quizzes.IntentoEnCurso(ctx, q.ID, insc.ID); err == nil {
		return s.vista(ctx, q, abierto)
	} else if !errors.Is(err, postgres.ErrNotFound) {
		return nil, err
	}

	previos, err := s.quizzes.ContarIntentos(ctx, q.ID, insc.ID)
	if err != nil {
		return nil, err
	}

	intento, err := quiz.NewAttempt(q, previos, s.snapshot(q), s.ahora())
	if err != nil {
		return nil, err
	}
	intento.EnrollmentID = insc.ID
	if err := s.quizzes.CrearIntento(ctx, intento); err != nil {
		return nil, err
	}
	return s.vista(ctx, q, intento)
}

// snapshot congela el orden y la clave correcta en el momento de iniciar.
//
// Es lo que hace reproducible la calificacion: si el profesor edita el quiz
// mientras alguien lo esta resolviendo, ese intento se sigue calificando
// contra lo que realmente vio el estudiante.
func (s *Service) snapshot(q *quiz.Quiz) quiz.Snapshot {
	snap := quiz.Snapshot{
		OptionOrder:    map[uuid.UUID][]uuid.UUID{},
		CorrectAnswers: map[uuid.UUID][]uuid.UUID{},
		Points:         map[uuid.UUID]float64{},
		QuestionType:   map[uuid.UUID]quiz.QuestionType{},
	}
	preguntas := make([]quiz.Question, len(q.Questions))
	copy(preguntas, q.Questions)
	if q.ShuffleQuestions {
		rand.Shuffle(len(preguntas), func(i, j int) { preguntas[i], preguntas[j] = preguntas[j], preguntas[i] })
	}
	for _, p := range preguntas {
		snap.QuestionOrder = append(snap.QuestionOrder, p.StableID)
		snap.Points[p.StableID] = p.Points
		snap.QuestionType[p.StableID] = p.Type
		for _, o := range p.Options {
			snap.OptionOrder[p.StableID] = append(snap.OptionOrder[p.StableID], o.StableID)
			if o.IsCorrect {
				snap.CorrectAnswers[p.StableID] = append(snap.CorrectAnswers[p.StableID], o.StableID)
			}
		}
	}
	return snap
}

// Guardar persiste el guardado parcial de una pregunta.
//
// Se validan pregunta y opciones contra el snapshot para que nadie responda
// algo que no estaba en su intento, y se rechaza si el intento ya no admite
// cambios.
func (s *Service) Guardar(ctx context.Context, actor *user.User, attemptID, preguntaStableID uuid.UUID, seleccion []uuid.UUID) error {
	intento, _, err := s.intentoPropio(ctx, actor, attemptID)
	if err != nil {
		return err
	}
	if intento.Status != quiz.AttemptInProgress {
		return quiz.ErrAttemptNotOpen
	}
	if intento.ExpiresAt != nil && s.ahora().After(*intento.ExpiresAt) {
		return quiz.ErrAttemptExpired
	}

	opciones, ok := intento.Snapshot.OptionOrder[preguntaStableID]
	if !ok {
		return ErrRespuestaInvalida
	}
	validas := map[uuid.UUID]struct{}{}
	for _, o := range opciones {
		validas[o] = struct{}{}
	}
	for _, elegida := range seleccion {
		if _, ok := validas[elegida]; !ok {
			return ErrRespuestaInvalida
		}
	}
	if intento.Snapshot.QuestionType[preguntaStableID] == quiz.QuestionSingle && len(seleccion) > 1 {
		return ErrRespuestaInvalida
	}
	return s.quizzes.GuardarRespuesta(ctx, attemptID, preguntaStableID, seleccion)
}

// Enviar califica el intento en servidor de forma idempotente.
//
// Reintentar con la misma clave devuelve la nota ya calculada en lugar de
// calificar otra vez, y dos envios concurrentes solo producen una calificacion
// porque el UPDATE exige que el intento siga en curso.
func (s *Service) Enviar(ctx context.Context, actor *user.User, attemptID uuid.UUID, claveIdempotencia string) (*VistaIntento, error) {
	intento, insc, err := s.intentoPropio(ctx, actor, attemptID)
	if err != nil {
		return nil, err
	}

	if claveIdempotencia != "" {
		if previo, err := s.quizzes.IntentoPorClaveIdempotencia(ctx, insc.ID, claveIdempotencia); err == nil {
			return s.vistaDeIntento(ctx, previo)
		} else if !errors.Is(err, postgres.ErrNotFound) {
			return nil, err
		}
	}

	if intento.Status == quiz.AttemptSubmitted {
		return nil, quiz.ErrAlreadySubmitted
	}

	respuestas, err := s.quizzes.Respuestas(ctx, attemptID)
	if err != nil {
		return nil, err
	}

	q, err := s.quizPorID(ctx, intento.QuizID)
	if err != nil {
		return nil, err
	}

	intento.SubmitIdempotencyKey = claveIdempotencia
	if err := intento.Grade(respuestas, q.PassScore, s.ahora()); err != nil {
		if errors.Is(err, quiz.ErrAttemptExpired) {
			// Expirar tambien cierra el intento: si no, quedaria abierto para
			// siempre y bloquearia los siguientes.
			_, _ = s.quizzes.CerrarIntento(ctx, intento)
		}
		return nil, err
	}

	calificado, err := s.quizzes.CerrarIntento(ctx, intento)
	if err != nil {
		return nil, err
	}
	if !calificado {
		// Otra peticion gano la carrera. Se devuelve lo que quedo guardado.
		return s.vistaDeIntentoPorID(ctx, attemptID)
	}

	// Aprobar un quiz puede completar el curso, asi que se reevalua el avance.
	if s.progreso != nil {
		if intento.Passed != nil && *intento.Passed {
			rec, err := s.courseRepo.GetRecursoPublicadoConMedia(ctx, q.ResourceID)
			if err != nil {
				return nil, err
			}
			if err := s.progreso.QuizAprobado(ctx, insc, rec.StableID); err != nil {
				return nil, err
			}
		} else if err := s.progreso.RecalcularCurso(ctx, insc); err != nil {
			return nil, err
		}
	}
	return s.vista(ctx, q, intento)
}

// Intento devuelve el estado de un intento propio, para reanudarlo o para
// consultar el resultado.
func (s *Service) Intento(ctx context.Context, actor *user.User, attemptID uuid.UUID) (*VistaIntento, error) {
	intento, _, err := s.intentoPropio(ctx, actor, attemptID)
	if err != nil {
		return nil, err
	}
	return s.vistaDeIntento(ctx, intento)
}

func (s *Service) autorizar(ctx context.Context, actor *user.User, resourceID uuid.UUID) (*quiz.Quiz, *enrollment.Enrollment, error) {
	rec, err := s.courseRepo.GetRecursoPublicadoConMedia(ctx, resourceID)
	if err != nil {
		return nil, nil, err
	}
	// Un recurso oculto responde como inexistente, para no delatar contenido
	// que el profesor todavia no quiere mostrar.
	if !rec.Visible || rec.Type != "quiz" {
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

	q, err := s.quizzes.GetByResource(ctx, resourceID)
	if err != nil {
		return nil, nil, err
	}
	return q, insc, nil
}

func (s *Service) intentoPropio(ctx context.Context, actor *user.User, attemptID uuid.UUID) (*quiz.Attempt, *enrollment.Enrollment, error) {
	intento, err := s.quizzes.IntentoPorID(ctx, attemptID)
	if err != nil {
		return nil, nil, err
	}
	insc, err := s.enrollments.GetByID(ctx, intento.EnrollmentID)
	if err != nil {
		return nil, nil, err
	}
	if insc.StudentID != actor.ID {
		return nil, nil, ErrIntentoAjeno
	}
	return intento, insc, nil
}

func (s *Service) quizPorID(ctx context.Context, quizID uuid.UUID) (*quiz.Quiz, error) {
	// El quiz se identifica por su recurso, asi que se resuelve al reves a
	// partir del intento ya cargado.
	return s.quizzes.PorID(ctx, quizID)
}

func (s *Service) vistaDeIntentoPorID(ctx context.Context, attemptID uuid.UUID) (*VistaIntento, error) {
	intento, err := s.quizzes.IntentoPorID(ctx, attemptID)
	if err != nil {
		return nil, err
	}
	return s.vistaDeIntento(ctx, intento)
}

func (s *Service) vistaDeIntento(ctx context.Context, intento *quiz.Attempt) (*VistaIntento, error) {
	q, err := s.quizPorID(ctx, intento.QuizID)
	if err != nil {
		return nil, err
	}
	return s.vista(ctx, q, intento)
}

// vista arma la proyeccion publica del intento.
//
// El texto de preguntas y opciones sale del quiz actual, pero el orden y el
// conjunto salen del snapshot: asi el estudiante ve lo mismo que al empezar
// aunque el autor haya reordenado el quiz mientras tanto.
func (s *Service) vista(ctx context.Context, q *quiz.Quiz, intento *quiz.Attempt) (*VistaIntento, error) {
	porStableID := map[uuid.UUID]quiz.Question{}
	for _, p := range q.Questions {
		porStableID[p.StableID] = p
	}

	v := &VistaIntento{
		AttemptID: intento.ID, QuizID: q.ID, Title: q.Title,
		AttemptNumber: intento.AttemptNumber, Status: string(intento.Status),
		StartedAt: intento.StartedAt, ExpiresAt: intento.ExpiresAt,
		Answers: map[string][]string{},
	}

	for _, preguntaID := range intento.Snapshot.QuestionOrder {
		p, ok := porStableID[preguntaID]
		if !ok {
			// La pregunta se borro tras iniciar el intento. Se omite del
			// enunciado, pero sigue contando en la nota segun el snapshot.
			continue
		}
		textos := map[uuid.UUID]string{}
		for _, o := range p.Options {
			textos[o.StableID] = o.TextMD
		}
		pv := PreguntaVisible{
			StableID: p.StableID, PromptMD: p.PromptMD,
			Type:   string(intento.Snapshot.QuestionType[preguntaID]),
			Points: intento.Snapshot.Points[preguntaID],
		}
		for _, opcionID := range intento.Snapshot.OptionOrder[preguntaID] {
			pv.Options = append(pv.Options, OpcionVisible{StableID: opcionID, TextMD: textos[opcionID]})
		}
		v.Questions = append(v.Questions, pv)
	}

	respuestas, err := s.quizzes.Respuestas(ctx, intento.ID)
	if err != nil {
		return nil, err
	}
	for pregunta, seleccion := range respuestas {
		ids := make([]string, 0, len(seleccion))
		for _, o := range seleccion {
			ids = append(ids, o.String())
		}
		v.Answers[pregunta.String()] = ids
	}

	if revelarNota(q.FeedbackPolicy, intento.Status) {
		v.Score, v.Passed = intento.Score, intento.Passed
	}
	return v, nil
}

// revelarNota aplica la politica de retroalimentacion configurada por el autor.
//
// "after_close" se trata hoy igual que "after_submit" porque el MVP no maneja
// fecha de cierre del quiz; cuando exista, aqui es donde se compara.
func revelarNota(politica quiz.FeedbackPolicy, estado quiz.AttemptStatus) bool {
	if politica == quiz.FeedbackNone {
		return false
	}
	if politica == quiz.FeedbackImmediate {
		return true
	}
	return estado == quiz.AttemptSubmitted
}
