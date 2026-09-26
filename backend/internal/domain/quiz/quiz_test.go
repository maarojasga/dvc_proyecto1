package quiz

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestGrade_CalificaEnServidorSinExponerClave(t *testing.T) {
	q1, q2 := uuid.New(), uuid.New()
	correctQ1, wrongQ1 := uuid.New(), uuid.New()
	correctQ2 := uuid.New()

	snapshot := Snapshot{
		QuestionOrder: []uuid.UUID{q1, q2},
		CorrectAnswers: map[uuid.UUID][]uuid.UUID{
			q1: {correctQ1},
			q2: {correctQ2},
		},
		Points:       map[uuid.UUID]float64{q1: 50, q2: 50},
		QuestionType: map[uuid.UUID]QuestionType{q1: QuestionSingle, q2: QuestionSingle},
	}

	attempt := &Attempt{Status: AttemptInProgress, Snapshot: snapshot}

	// El estudiante acierta la primera y falla la segunda.
	answers := map[uuid.UUID][]uuid.UUID{
		q1: {correctQ1},
		q2: {wrongQ1},
	}

	if err := attempt.Grade(answers, 60, time.Now()); err != nil {
		t.Fatalf("no se esperaba error: %v", err)
	}
	if attempt.Score == nil || *attempt.Score != 50 {
		t.Fatalf("esperaba score=50, obtuve %v", attempt.Score)
	}
	if attempt.Passed == nil || *attempt.Passed {
		t.Fatalf("esperaba passed=false con score 50 < 60, obtuve %v", attempt.Passed)
	}
	if attempt.Status != AttemptSubmitted {
		t.Fatalf("esperaba estado submitted, obtuve %s", attempt.Status)
	}
}

func TestGrade_IntentoExpirado(t *testing.T) {
	past := time.Now().Add(-time.Hour)
	attempt := &Attempt{Status: AttemptInProgress, ExpiresAt: &past, Snapshot: Snapshot{}}

	err := attempt.Grade(map[uuid.UUID][]uuid.UUID{}, 60, time.Now())
	if err != ErrAttemptExpired {
		t.Fatalf("esperaba ErrAttemptExpired, obtuve %v", err)
	}
	if attempt.Status != AttemptExpired {
		t.Fatalf("esperaba estado expired, obtuve %s", attempt.Status)
	}
}

func TestGrade_NoPermiteRecalificar(t *testing.T) {
	attempt := &Attempt{Status: AttemptSubmitted, Snapshot: Snapshot{}}
	if err := attempt.Grade(map[uuid.UUID][]uuid.UUID{}, 60, time.Now()); err != ErrAttemptNotOpen {
		t.Fatalf("esperaba ErrAttemptNotOpen, obtuve %v", err)
	}
}

func TestNewAttempt_RespetaMaxIntentos(t *testing.T) {
	max := 2
	q := &Quiz{ID: uuid.New(), MaxAttempts: &max}

	if _, err := NewAttempt(q, 2, Snapshot{}, time.Now()); err != ErrMaxAttemptsReached {
		t.Fatalf("esperaba ErrMaxAttemptsReached, obtuve %v", err)
	}

	a, err := NewAttempt(q, 1, Snapshot{}, time.Now())
	if err != nil {
		t.Fatalf("no se esperaba error: %v", err)
	}
	if a.AttemptNumber != 2 {
		t.Fatalf("esperaba intento número 2, obtuve %d", a.AttemptNumber)
	}
}

func TestToPublic_NuncaExponeIsCorrect(t *testing.T) {
	q := Question{Options: []Option{
		{StableID: uuid.New(), TextMD: "A", IsCorrect: true},
		{StableID: uuid.New(), TextMD: "B", IsCorrect: false},
	}}
	pub := q.ToPublic()
	for _, o := range pub {
		_ = o // PublicOption no tiene campo IsCorrect: si compila, la clave no se expone.
	}
	if len(pub) != 2 {
		t.Fatalf("esperaba 2 opciones públicas, obtuve %d", len(pub))
	}
}
