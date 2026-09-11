package postgres

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/DES-SOLUCIONES-CLOUD/proyecto-1/backend/internal/domain/badge"
)

// BadgeRepo persiste las insignias emitidas al aprobar un curso.
type BadgeRepo struct{ pool *pgxpool.Pool }

func NewBadgeRepo(pool *pgxpool.Pool) *BadgeRepo { return &BadgeRepo{pool: pool} }

const camposInsignia = `id, enrollment_id, course_id, student_id, verification_code,
	coalesce(image_object_key, ''), issued_at, revoked_at, coalesce(revoked_reason, '')`

// Emitir crea la insignia de forma idempotente.
//
// La unicidad de enrollment_id la garantiza la base: si dos peticiones
// concurrentes intentan emitir, una inserta y la otra no altera nada, y ambas
// terminan devolviendo la misma insignia. Por eso el conflicto no es un error.
func (r *BadgeRepo) Emitir(ctx context.Context, b *badge.Badge) (*badge.Badge, error) {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO badges (id, enrollment_id, course_id, student_id, verification_code, issued_at)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (enrollment_id) DO NOTHING`,
		b.ID, b.EnrollmentID, b.CourseID, b.StudentID, b.VerificationCode, b.IssuedAt)
	if err != nil {
		return nil, err
	}
	return r.PorInscripcion(ctx, b.EnrollmentID)
}

func (r *BadgeRepo) PorInscripcion(ctx context.Context, enrollmentID uuid.UUID) (*badge.Badge, error) {
	return r.leer(ctx, `SELECT `+camposInsignia+` FROM badges WHERE enrollment_id = $1`, enrollmentID)
}

// PorCodigo resuelve la URL publica de verificacion.
func (r *BadgeRepo) PorCodigo(ctx context.Context, codigo string) (*badge.Badge, error) {
	return r.leer(ctx, `SELECT `+camposInsignia+` FROM badges WHERE verification_code = $1`, codigo)
}

func (r *BadgeRepo) ListarPorEstudiante(ctx context.Context, studentID uuid.UUID) ([]*badge.Badge, error) {
	filas, err := r.pool.Query(ctx,
		`SELECT `+camposInsignia+` FROM badges WHERE student_id = $1 ORDER BY issued_at DESC`, studentID)
	if err != nil {
		return nil, err
	}
	defer filas.Close()

	var out []*badge.Badge
	for filas.Next() {
		b, err := escanearInsignia(filas)
		if err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, filas.Err()
}

// Revocar invalida la insignia dejando constancia del motivo. La URL de
// verificacion sigue resolviendo, pero reporta el estado revocado.
func (r *BadgeRepo) Revocar(ctx context.Context, b *badge.Badge) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE badges SET revoked_at = $2, revoked_reason = $3 WHERE id = $1`,
		b.ID, b.RevokedAt, b.RevokedReason)
	return err
}

func (r *BadgeRepo) leer(ctx context.Context, sql string, args ...any) (*badge.Badge, error) {
	b, err := escanearInsignia(r.pool.QueryRow(ctx, sql, args...))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return b, err
}

type escaneable interface{ Scan(dest ...any) error }

func escanearInsignia(s escaneable) (*badge.Badge, error) {
	var b badge.Badge
	err := s.Scan(&b.ID, &b.EnrollmentID, &b.CourseID, &b.StudentID,
		&b.VerificationCode, &b.ImageObjectKey, &b.IssuedAt, &b.RevokedAt, &b.RevokedReason)
	if err != nil {
		return nil, err
	}
	return &b, nil
}
