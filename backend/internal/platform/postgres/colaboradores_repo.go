package postgres

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ColaboradoresRepo gestiona quién puede editar un curso además de su dueño.
type ColaboradoresRepo struct{ pool *pgxpool.Pool }

func NewColaboradoresRepo(pool *pgxpool.Pool) *ColaboradoresRepo {
	return &ColaboradoresRepo{pool: pool}
}

// Colaborador es un profesor con acceso de edición a un curso ajeno.
type Colaborador struct {
	UserID   uuid.UUID `json:"user_id"`
	Email    string    `json:"email"`
	FullName string    `json:"full_name"`
	Role     string    `json:"role"`
	AddedAt  time.Time `json:"added_at"`
}

// PuedeEditar responde si el usuario es colaborador del curso.
//
// Es una consulta por petición en el camino de la autorización, así que va
// directa contra la clave primaria (course_id, user_id): no hay recorrido de
// tabla ni lista que cargar en memoria.
func (r *ColaboradoresRepo) PuedeEditar(ctx context.Context, courseID, userID uuid.UUID) (bool, error) {
	var existe bool
	err := r.pool.QueryRow(ctx, `
		SELECT EXISTS (
		    SELECT 1 FROM course_collaborators WHERE course_id = $1 AND user_id = $2
		)`, courseID, userID).Scan(&existe)
	return existe, err
}

func (r *ColaboradoresRepo) Listar(ctx context.Context, courseID uuid.UUID) ([]Colaborador, error) {
	filas, err := r.pool.Query(ctx, `
		SELECT cc.user_id, u.email, u.full_name, cc.role, cc.added_at
		  FROM course_collaborators cc
		  JOIN users u ON u.id = cc.user_id
		 WHERE cc.course_id = $1
		 ORDER BY u.email`, courseID)
	if err != nil {
		return nil, err
	}
	defer filas.Close()

	out := []Colaborador{}
	for filas.Next() {
		var c Colaborador
		if err := filas.Scan(&c.UserID, &c.Email, &c.FullName, &c.Role, &c.AddedAt); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, filas.Err()
}

// Agregar da acceso de edición. Repetirlo no es un error: el resultado que
// quiere quien lo pide es que esa persona tenga acceso, y ya lo tiene.
func (r *ColaboradoresRepo) Agregar(ctx context.Context, courseID, userID, agregadoPor uuid.UUID) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO course_collaborators (course_id, user_id, added_by)
		VALUES ($1, $2, $3)
		ON CONFLICT (course_id, user_id) DO NOTHING`, courseID, userID, agregadoPor)
	return err
}

func (r *ColaboradoresRepo) Quitar(ctx context.Context, courseID, userID uuid.UUID) error {
	tag, err := r.pool.Exec(ctx,
		`DELETE FROM course_collaborators WHERE course_id = $1 AND user_id = $2`, courseID, userID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// CursosDondeColabora son los cursos ajenos que este profesor puede editar.
// Los necesita su pantalla de autoría: sin esto, un colaborador tendría acceso
// pero ninguna forma de llegar al curso.
func (r *ColaboradoresRepo) CursosDondeColabora(ctx context.Context, userID uuid.UUID) ([]uuid.UUID, error) {
	filas, err := r.pool.Query(ctx,
		`SELECT course_id FROM course_collaborators WHERE user_id = $1`, userID)
	if err != nil {
		return nil, err
	}
	defer filas.Close()

	var out []uuid.UUID
	for filas.Next() {
		var id uuid.UUID
		if err := filas.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, filas.Err()
}
