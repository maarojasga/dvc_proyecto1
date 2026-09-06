package postgres

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ProgressRepo persiste el avance por recurso.
//
// De momento solo cubre la posición de reproducción, que es una comodidad
// para reanudar. El avance propiamente dicho (heartbeats, permanencia,
// finalización) se calcula en el servidor y llegará con su propio alcance.
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
