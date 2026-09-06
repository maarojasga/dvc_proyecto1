// Package quiz contiene las reglas de evaluación de selección múltiple:
// intentos, guardado parcial, calificación en servidor y retroalimentación
// configurable. La clave correcta nunca debe serializarse hacia el cliente;
// por eso Option.IsCorrect se omite explícitamente en cualquier DTO público.
package quiz

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

type FeedbackPolicy string

const (
	FeedbackImmediate   FeedbackPolicy = "immediate"
	FeedbackAfterSubmit FeedbackPolicy = "after_submit"
	FeedbackAfterClose  FeedbackPolicy = "after_close"
	FeedbackNone        FeedbackPolicy = "none"
)

type AttemptStatus string

const (
	AttemptInProgress AttemptStatus = "in_progress"
	AttemptSubmitted  AttemptStatus = "submitted"
	AttemptExpired    AttemptStatus = "expired"
)

type QuestionType string

const (
	QuestionSingle   QuestionType = "single"
	QuestionMultiple QuestionType = "multiple"
)

var (
	ErrMaxAttemptsReached = errors.New("quiz: se alcanzó el número máximo de intentos")
	ErrAttemptExpired     = errors.New("quiz: el intento expiró")
	ErrAttemptNotOpen     = errors.New("quiz: el intento no está en progreso")
	ErrAlreadySubmitted   = errors.New("quiz: el intento ya fue calificado")
)

// Quiz es la configuración de una evaluación asociada a un recurso.
type Quiz struct {
	ID               uuid.UUID
	ResourceID       uuid.UUID
	Title            string
	TimeLimitSeconds *int
	MaxAttempts      *int
	PassScore        float64
	FeedbackPolicy   FeedbackPolicy
	ShuffleQuestions bool
	Questions        []Question
}

type Question struct {
	ID       uuid.UUID
	StableID uuid.UUID
	Position int
	PromptMD string
	Type     QuestionType
	Points   float64
	Options  []Option
}

type Option struct {
	ID        uuid.UUID
	StableID  uuid.UUID
	Position  int
	TextMD    string
	IsCorrect bool
}

// PublicOption es la proyección segura de una opción, sin IsCorrect.
type PublicOption struct {
	StableID uuid.UUID
	Position int
	TextMD   string
}

// ToPublic despoja la clave correcta antes de enviar la pregunta al cliente.
func (q *Question) ToPublic() []PublicOption {
	out := make([]PublicOption, 0, len(q.Options))
	for _, o := range q.Options {
		out = append(out, PublicOption{StableID: o.StableID, Position: o.Position, TextMD: o.TextMD})
	}
	return out
}

// Attempt es un intento de un estudiante sobre un quiz. Snapshot congela el
// orden y los identificadores estables de preguntas/opciones mostrados,
// para que la calificación sea reproducible aunque el autor edite el quiz
// después.
type Attempt struct {
	ID                   uuid.UUID
	QuizID               uuid.UUID
	EnrollmentID         uuid.UUID
	AttemptNumber        int
	Status               AttemptStatus
	Snapshot             Snapshot
	StartedAt            time.Time
	SubmittedAt          *time.Time
	ExpiresAt            *time.Time
	Score                *float64
	Passed               *bool
	SubmitIdempotencyKey string
}

// Snapshot es la copia congelada de preguntas/opciones (sin IsCorrect en la
// parte servida al cliente, pero el servidor conserva por separado el mapa
// de respuestas correctas para calificar).
type Snapshot struct {
	QuestionOrder  []uuid.UUID               // stable_id de preguntas, en el orden mostrado
	OptionOrder    map[uuid.UUID][]uuid.UUID // stable_id pregunta -> stable_id opciones en orden mostrado
	CorrectAnswers map[uuid.UUID][]uuid.UUID // solo en el servidor: stable_id pregunta -> stable_id opciones correctas
	Points         map[uuid.UUID]float64
	QuestionType   map[uuid.UUID]QuestionType
}

// NewAttempt valida el número de intentos disponibles y construye un nuevo
// intento en progreso a partir de un snapshot ya armado por el servicio.
func NewAttempt(quiz *Quiz, previousAttempts int, snapshot Snapshot, now time.Time) (*Attempt, error) {
	if quiz.MaxAttempts != nil && previousAttempts >= *quiz.MaxAttempts {
		return nil, ErrMaxAttemptsReached
	}
	a := &Attempt{
		ID:            uuid.New(),
		QuizID:        quiz.ID,
		AttemptNumber: previousAttempts + 1,
		Status:        AttemptInProgress,
		Snapshot:      snapshot,
		StartedAt:     now,
	}
	if quiz.TimeLimitSeconds != nil {
		exp := now.Add(time.Duration(*quiz.TimeLimitSeconds) * time.Second)
		a.ExpiresAt = &exp
	}
	return a, nil
}

// Grade califica el intento en servidor a partir de las respuestas
// seleccionadas (stable_id de pregunta -> stable_id de opciones elegidas),
// sin que la clave correcta haya salido nunca del servidor.
func (a *Attempt) Grade(answers map[uuid.UUID][]uuid.UUID, passScore float64, now time.Time) error {
	if a.Status != AttemptInProgress {
		return ErrAttemptNotOpen
	}
	if a.ExpiresAt != nil && now.After(*a.ExpiresAt) {
		a.Status = AttemptExpired
		return ErrAttemptExpired
	}

	totalPoints := 0.0
	earned := 0.0
	for _, qStableID := range a.Snapshot.QuestionOrder {
		points := a.Snapshot.Points[qStableID]
		totalPoints += points

		correct := toSet(a.Snapshot.CorrectAnswers[qStableID])
		selected := toSet(answers[qStableID])
		if setsEqual(correct, selected) {
			earned += points
		}
	}

	score := 0.0
	if totalPoints > 0 {
		score = (earned / totalPoints) * 100
	}
	passed := score >= passScore

	a.Score = &score
	a.Passed = &passed
	a.Status = AttemptSubmitted
	a.SubmittedAt = &now
	return nil
}

func toSet(ids []uuid.UUID) map[uuid.UUID]struct{} {
	s := make(map[uuid.UUID]struct{}, len(ids))
	for _, id := range ids {
		s[id] = struct{}{}
	}
	return s
}

func setsEqual(a, b map[uuid.UUID]struct{}) bool {
	if len(a) != len(b) {
		return false
	}
	for k := range a {
		if _, ok := b[k]; !ok {
			return false
		}
	}
	return true
}
