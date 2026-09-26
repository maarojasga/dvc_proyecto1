package progress

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestValidateHeartbeat_RechazaDemasiadoRapido(t *testing.T) {
	last := time.Now()
	now := last.Add(1 * time.Second) // menor a MinHeartbeatInterval/2

	if _, err := ValidateHeartbeat(last, now); err != ErrEventRejected {
		t.Fatalf("esperaba ErrEventRejected, obtuve %v", err)
	}
}

func TestValidateHeartbeat_AcreditaDentroDelRango(t *testing.T) {
	last := time.Now()
	now := last.Add(20 * time.Second)

	credited, err := ValidateHeartbeat(last, now)
	if err != nil {
		t.Fatalf("no se esperaba error: %v", err)
	}
	if credited != 20*time.Second {
		t.Fatalf("esperaba acreditar 20s, obtuve %v", credited)
	}
}

func TestValidateHeartbeat_TopaCreditoMaximo(t *testing.T) {
	last := time.Now()
	now := last.Add(10 * time.Minute) // heartbeat espaciado artificialmente

	credited, err := ValidateHeartbeat(last, now)
	if err != nil {
		t.Fatalf("no se esperaba error: %v", err)
	}
	if credited != MaxCreditPerEvent {
		t.Fatalf("esperaba tope de %v, obtuve %v", MaxCreditPerEvent, credited)
	}
}

func TestCourseCompletion_CalculaPorcentajeSobreObligatorios(t *testing.T) {
	r1, r2, r3 := uuid.New(), uuid.New(), uuid.New()
	required := []uuid.UUID{r1, r2, r3}
	states := map[uuid.UUID]*ResourceState{
		r1: {Status: "completed"},
		r2: {Status: "completed"},
		r3: {Status: "in_progress"},
	}

	percent, completed := CourseCompletion(required, states, 100)
	if completed {
		t.Fatal("no esperaba completar con un recurso obligatorio pendiente")
	}
	want := float64(2) / float64(3) * 100
	if percent != want {
		t.Fatalf("esperaba %.2f%%, obtuve %.2f%%", want, percent)
	}

	percent, completed = CourseCompletion(required, states, 60)
	if !completed {
		t.Fatalf("esperaba completar con umbral 60%% y %.2f%% logrado", percent)
	}
}
