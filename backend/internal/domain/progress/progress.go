// Package progress modela el avance de un estudiante sobre los recursos de
// un curso a partir de señales verificadas en servidor (apertura, heartbeat,
// cierre), nunca de porcentajes reportados por el cliente.
package progress

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

// EventType es el tipo de señal de progreso reportada por el cliente y
// validada por el servidor.
type EventType string

const (
	EventOpen      EventType = "open"
	EventHeartbeat EventType = "heartbeat"
	EventClose     EventType = "close"
)

// MinHeartbeatInterval y MaxHeartbeatInterval acotan la ventana aceptable
// entre heartbeats; fuera de este rango la señal se descarta como inválida
// (demasiado frecuente sugiere manipulación, demasiado espaciada no permite
// acreditar permanencia continua).
const (
	MinHeartbeatInterval = 5 * time.Second
	MaxHeartbeatInterval = 90 * time.Second
	MaxCreditPerEvent     = 60 * time.Second
)

var ErrEventRejected = errors.New("progress: la señal de progreso fue rechazada por implausible")

// Event es una señal cruda de progreso, persistida para auditoría.
type Event struct {
	ID               uuid.UUID
	EnrollmentID     uuid.UUID
	ResourceStableID uuid.UUID
	Type             EventType
	DurationSeconds  int
	OccurredAt       time.Time
}

// ResourceState es el estado materializado de avance sobre un recurso.
type ResourceState struct {
	EnrollmentID     uuid.UUID
	ResourceStableID uuid.UUID
	Status           string // not_started | in_progress | completed
	TimeSpentSeconds int
	FirstOpenedAt    *time.Time
	CompletedAt      *time.Time
}

// ValidateHeartbeat aplica el rechazo de señales implausibles descrito en la
// condición "Progreso verificable": una duración reportada fuera del rango
// aceptado se descarta y debe auditarse, en lugar de acreditarse tal cual.
func ValidateHeartbeat(lastEventAt time.Time, now time.Time) (credited time.Duration, err error) {
	elapsed := now.Sub(lastEventAt)
	if elapsed <= 0 || elapsed < MinHeartbeatInterval/2 {
		return 0, ErrEventRejected
	}
	if elapsed > MaxCreditPerEvent {
		// Se acredita como máximo MaxCreditPerEvent para no premiar pestañas
		// abandonadas o heartbeats espaciados artificialmente.
		return MaxCreditPerEvent, nil
	}
	return elapsed, nil
}

// ApplyHeartbeat acumula tiempo válido sobre el estado del recurso y marca
// el inicio si es la primera señal.
func (s *ResourceState) ApplyHeartbeat(credited time.Duration, now time.Time) {
	if s.FirstOpenedAt == nil {
		s.FirstOpenedAt = &now
	}
	s.TimeSpentSeconds += int(credited.Seconds())
	if s.Status == "not_started" {
		s.Status = "in_progress"
	}
}

// MarkCompleted marca el recurso como completado (heurística de progreso
// mínimo o evento explícito de cierre tras consumo suficiente).
func (s *ResourceState) MarkCompleted(now time.Time) {
	if s.Status == "completed" {
		return
	}
	s.Status = "completed"
	s.CompletedAt = &now
}

// CourseCompletion calcula si el porcentaje de recursos obligatorios
// completados alcanza el umbral configurado por el curso.
func CourseCompletion(requiredResourceIDs []uuid.UUID, states map[uuid.UUID]*ResourceState, requiredPct float64) (percent float64, completed bool) {
	if len(requiredResourceIDs) == 0 {
		return 100, true
	}
	done := 0
	for _, id := range requiredResourceIDs {
		if st, ok := states[id]; ok && st.Status == "completed" {
			done++
		}
	}
	percent = (float64(done) / float64(len(requiredResourceIDs))) * 100
	return percent, percent >= requiredPct
}
