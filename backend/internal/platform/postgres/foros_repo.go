package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ForosRepo persiste los foros asíncronos de un curso.
type ForosRepo struct{ pool *pgxpool.Pool }

func NewForosRepo(pool *pgxpool.Pool) *ForosRepo { return &ForosRepo{pool: pool} }

// Hilo es una conversación dentro de un curso.
type Hilo struct {
	ID       uuid.UUID `json:"id"`
	CourseID uuid.UUID `json:"course_id"`
	// RecursoStableID ata el hilo a una lección concreta. Es el stable_id y no
	// el identificador de fila para que la conversación sobreviva a la
	// publicación de una versión nueva.
	RecursoStableID *uuid.UUID `json:"resource_stable_id,omitempty"`
	AutorID         *uuid.UUID `json:"author_id,omitempty"`
	AutorNombre     string     `json:"author_name,omitempty"`
	Titulo          string     `json:"title"`
	CuerpoMD        string     `json:"body_md"`
	Bloqueado       bool       `json:"locked"`
	Respuestas      int        `json:"replies"`
	CreadoEn        time.Time  `json:"created_at"`
	UltimaActividad time.Time  `json:"last_activity_at"`
}

// Respuesta es un mensaje dentro de un hilo.
type Respuesta struct {
	ID          uuid.UUID  `json:"id"`
	ThreadID    uuid.UUID  `json:"thread_id"`
	AutorID     *uuid.UUID `json:"author_id,omitempty"`
	AutorNombre string     `json:"author_name,omitempty"`
	CuerpoMD    string     `json:"body_md"`
	CreadoEn    time.Time  `json:"created_at"`
	Eliminado   bool       `json:"deleted"`
}

// CrearHilo abre una conversación.
func (r *ForosRepo) CrearHilo(ctx context.Context, h *Hilo) error {
	return r.pool.QueryRow(ctx, `
		INSERT INTO forum_threads (course_id, resource_stable_id, author_id, title, body_md)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, created_at, last_activity_at`,
		h.CourseID, h.RecursoStableID, h.AutorID, h.Titulo, h.CuerpoMD).
		Scan(&h.ID, &h.CreadoEn, &h.UltimaActividad)
}

// ListarHilos devuelve las conversaciones de un curso, la más activa primero.
//
// El recuento de respuestas va en la misma consulta: pedirlo por hilo
// convertiría un listado de veinte en veintiuna consultas.
func (r *ForosRepo) ListarHilos(ctx context.Context, courseID uuid.UUID, recurso *uuid.UUID) ([]Hilo, error) {
	filas, err := r.pool.Query(ctx, `
		SELECT t.id, t.course_id, t.resource_stable_id, t.author_id,
		       coalesce(u.full_name, ''), t.title, t.body_md, t.locked,
		       (SELECT count(*) FROM forum_posts p
		         WHERE p.thread_id = t.id AND p.deleted_at IS NULL),
		       t.created_at, t.last_activity_at
		  FROM forum_threads t
		  LEFT JOIN users u ON u.id = t.author_id
		 WHERE t.course_id = $1
		   AND ($2::uuid IS NULL OR t.resource_stable_id = $2)
		 ORDER BY t.last_activity_at DESC
		 LIMIT 100`, courseID, recurso)
	if err != nil {
		return nil, err
	}
	defer filas.Close()

	out := []Hilo{}
	for filas.Next() {
		var h Hilo
		if err := filas.Scan(&h.ID, &h.CourseID, &h.RecursoStableID, &h.AutorID,
			&h.AutorNombre, &h.Titulo, &h.CuerpoMD, &h.Bloqueado,
			&h.Respuestas, &h.CreadoEn, &h.UltimaActividad); err != nil {
			return nil, err
		}
		out = append(out, h)
	}
	return out, filas.Err()
}

func (r *ForosRepo) Hilo(ctx context.Context, id uuid.UUID) (*Hilo, error) {
	var h Hilo
	err := r.pool.QueryRow(ctx, `
		SELECT t.id, t.course_id, t.resource_stable_id, t.author_id,
		       coalesce(u.full_name, ''), t.title, t.body_md, t.locked,
		       0, t.created_at, t.last_activity_at
		  FROM forum_threads t
		  LEFT JOIN users u ON u.id = t.author_id
		 WHERE t.id = $1`, id).
		Scan(&h.ID, &h.CourseID, &h.RecursoStableID, &h.AutorID, &h.AutorNombre,
			&h.Titulo, &h.CuerpoMD, &h.Bloqueado, &h.Respuestas, &h.CreadoEn, &h.UltimaActividad)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return &h, err
}

// Responder añade un mensaje y actualiza la actividad del hilo.
//
// Las dos escrituras van en una transacción: un hilo cuya última actividad no
// refleje su última respuesta se hunde en el listado y la conversación muere
// sin que nadie la vea.
func (r *ForosRepo) Responder(ctx context.Context, p *Respuesta) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := tx.QueryRow(ctx, `
		INSERT INTO forum_posts (thread_id, author_id, body_md)
		VALUES ($1, $2, $3) RETURNING id, created_at`,
		p.ThreadID, p.AutorID, p.CuerpoMD).Scan(&p.ID, &p.CreadoEn); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx,
		`UPDATE forum_threads SET last_activity_at = now() WHERE id = $1`, p.ThreadID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// Respuestas devuelve los mensajes de un hilo en orden cronológico.
//
// Las eliminadas se devuelven marcadas y sin cuerpo, no se omiten: quitar un
// mensaje intermedio de una conversación deja sin sentido a los que le
// respondían.
func (r *ForosRepo) Respuestas(ctx context.Context, threadID uuid.UUID) ([]Respuesta, error) {
	filas, err := r.pool.Query(ctx, `
		SELECT p.id, p.thread_id, p.author_id, coalesce(u.full_name, ''),
		       CASE WHEN p.deleted_at IS NULL THEN p.body_md ELSE '' END,
		       p.created_at, p.deleted_at IS NOT NULL
		  FROM forum_posts p
		  LEFT JOIN users u ON u.id = p.author_id
		 WHERE p.thread_id = $1
		 ORDER BY p.created_at`, threadID)
	if err != nil {
		return nil, err
	}
	defer filas.Close()

	out := []Respuesta{}
	for filas.Next() {
		var p Respuesta
		if err := filas.Scan(&p.ID, &p.ThreadID, &p.AutorID, &p.AutorNombre,
			&p.CuerpoMD, &p.CreadoEn, &p.Eliminado); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, filas.Err()
}

// EliminarRespuesta la oculta conservando su lugar en la conversación.
func (r *ForosRepo) EliminarRespuesta(ctx context.Context, postID uuid.UUID) (autorID *uuid.UUID, err error) {
	err = r.pool.QueryRow(ctx, `
		UPDATE forum_posts SET deleted_at = now()
		 WHERE id = $1 AND deleted_at IS NULL
		 RETURNING author_id`, postID).Scan(&autorID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return autorID, err
}

// AutorDeRespuesta resuelve de quién es un mensaje, para decidir si quien pide
// borrarlo puede hacerlo.
func (r *ForosRepo) AutorDeRespuesta(ctx context.Context, postID uuid.UUID) (uuid.UUID, uuid.UUID, error) {
	var autor, curso uuid.UUID
	err := r.pool.QueryRow(ctx, `
		SELECT coalesce(p.author_id, '00000000-0000-0000-0000-000000000000'::uuid), t.course_id
		  FROM forum_posts p JOIN forum_threads t ON t.id = p.thread_id
		 WHERE p.id = $1`, postID).Scan(&autor, &curso)
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, uuid.Nil, ErrNotFound
	}
	return autor, curso, err
}

// BloquearHilo cierra o reabre una conversación.
func (r *ForosRepo) BloquearHilo(ctx context.Context, threadID uuid.UUID, bloqueado bool) error {
	tag, err := r.pool.Exec(ctx,
		`UPDATE forum_threads SET locked = $2 WHERE id = $1`, threadID, bloqueado)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
