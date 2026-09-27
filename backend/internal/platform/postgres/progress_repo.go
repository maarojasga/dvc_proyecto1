package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/DES-SOLUCIONES-CLOUD/proyecto-1/backend/internal/domain/progress"
)

// ProgressRepo persiste el avance por recurso: la posición de reproducción,
// que es una comodidad para reanudar, y el avance propiamente dicho
// (eventos crudos y estado materializado), que calcula el servidor a partir
// de señales verificadas y nunca de porcentajes enviados por el cliente.
type ProgressRepo struct{ pool *pgxpool.Pool }

func NewProgressRepo(pool *pgxpool.Pool) *ProgressRepo { return &ProgressRepo{pool: pool} }

// GuardarPosicion registra el segundo donde reanudar.
//
// La fila se identifica por inscripción y stable_id del recurso, no por el id
// de la fila de recursos: el stable_id sobrevive a la publicación de una
// versión nueva, así que la posición se conserva cuando el curso se actualiza.
func (r *ProgressRepo) GuardarPosicion(ctx context.Context, enrollmentID, resourceStableID uuid.UUID, segundos int) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO resource_progress (enrollment_id, resource_stable_id, last_position_seconds, updated_at)
		VALUES ($1, $2, $3, now())
		ON CONFLICT (enrollment_id, resource_stable_id) DO UPDATE
		   SET last_position_seconds = EXCLUDED.last_position_seconds,
		       updated_at = now()`,
		enrollmentID, resourceStableID, segundos)
	return err
}

// Posicion devuelve el segundo donde reanudar, o 0 si no hay nada registrado.
func (r *ProgressRepo) Posicion(ctx context.Context, enrollmentID, resourceStableID uuid.UUID) (int, error) {
	var segundos int
	err := r.pool.QueryRow(ctx, `
		SELECT last_position_seconds FROM resource_progress
		 WHERE enrollment_id = $1 AND resource_stable_id = $2`,
		enrollmentID, resourceStableID).Scan(&segundos)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, nil
	}
	return segundos, err
}

// RegistrarEvento deja la señal cruda en la bitácora.
//
// Se persiste siempre, incluso cuando el servidor la descarta por implausible,
// porque la condición "Progreso verificable" exige auditar los intentos de
// manipulación, no solo rechazarlos.
func (r *ProgressRepo) RegistrarEvento(ctx context.Context, e *progress.Event) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO progress_events
		    (id, enrollment_id, resource_stable_id, event_type, duration_seconds, occurred_at)
		VALUES ($1, $2, $3, $4, $5, $6)`,
		e.ID, e.EnrollmentID, e.ResourceStableID, string(e.Type), e.DurationSeconds, e.OccurredAt)
	return err
}

// UltimoEvento devuelve cuándo se recibió la señal anterior sobre un recurso.
// Es la base para acreditar permanencia: el crédito sale de la distancia entre
// dos señales medidas por el servidor, no de lo que declare el cliente.
func (r *ProgressRepo) UltimoEvento(ctx context.Context, enrollmentID, resourceStableID uuid.UUID) (time.Time, bool, error) {
	var cuando time.Time
	err := r.pool.QueryRow(ctx, `
		SELECT occurred_at FROM progress_events
		 WHERE enrollment_id = $1 AND resource_stable_id = $2
		 ORDER BY occurred_at DESC LIMIT 1`,
		enrollmentID, resourceStableID).Scan(&cuando)
	if errors.Is(err, pgx.ErrNoRows) {
		return time.Time{}, false, nil
	}
	if err != nil {
		return time.Time{}, false, err
	}
	return cuando, true, nil
}

// EstadoRecurso lee el avance materializado, devolviendo el estado inicial
// cuando el estudiante todavía no ha abierto el recurso.
func (r *ProgressRepo) EstadoRecurso(ctx context.Context, enrollmentID, resourceStableID uuid.UUID) (*progress.ResourceState, error) {
	st := &progress.ResourceState{
		EnrollmentID: enrollmentID, ResourceStableID: resourceStableID, Status: "not_started",
	}
	err := r.pool.QueryRow(ctx, `
		SELECT status, time_spent_seconds, first_opened_at, completed_at
		  FROM resource_progress
		 WHERE enrollment_id = $1 AND resource_stable_id = $2`,
		enrollmentID, resourceStableID).
		Scan(&st.Status, &st.TimeSpentSeconds, &st.FirstOpenedAt, &st.CompletedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return st, nil
	}
	if err != nil {
		return nil, err
	}
	return st, nil
}

// GuardarEstado materializa el avance calculado.
//
// No toca last_position_seconds: esa columna la escribe GuardarPosicion y es
// una comodidad del reproductor, ajena al avance acreditado.
func (r *ProgressRepo) GuardarEstado(ctx context.Context, st *progress.ResourceState) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO resource_progress
		    (enrollment_id, resource_stable_id, status, time_spent_seconds,
		     first_opened_at, completed_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, now())
		ON CONFLICT (enrollment_id, resource_stable_id) DO UPDATE
		   SET status = EXCLUDED.status,
		       time_spent_seconds = EXCLUDED.time_spent_seconds,
		       first_opened_at = coalesce(resource_progress.first_opened_at, EXCLUDED.first_opened_at),
		       completed_at = coalesce(resource_progress.completed_at, EXCLUDED.completed_at),
		       updated_at = now()`,
		st.EnrollmentID, st.ResourceStableID, st.Status, st.TimeSpentSeconds,
		st.FirstOpenedAt, st.CompletedAt)
	return err
}

// EstadosPorInscripcion carga todo el avance de una inscripción, indexado por
// stable_id, que es lo que necesita el cálculo de finalización del curso.
func (r *ProgressRepo) EstadosPorInscripcion(ctx context.Context, enrollmentID uuid.UUID) (map[uuid.UUID]*progress.ResourceState, error) {
	filas, err := r.pool.Query(ctx, `
		SELECT resource_stable_id, status, time_spent_seconds, first_opened_at, completed_at
		  FROM resource_progress WHERE enrollment_id = $1`, enrollmentID)
	if err != nil {
		return nil, err
	}
	defer filas.Close()

	out := map[uuid.UUID]*progress.ResourceState{}
	for filas.Next() {
		st := &progress.ResourceState{EnrollmentID: enrollmentID}
		if err := filas.Scan(&st.ResourceStableID, &st.Status, &st.TimeSpentSeconds,
			&st.FirstOpenedAt, &st.CompletedAt); err != nil {
			return nil, err
		}
		out[st.ResourceStableID] = st
	}
	return out, filas.Err()
}
