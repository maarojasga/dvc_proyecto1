package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// SubtitulosRepo persiste las pistas de subtítulos y sus transcripciones.
type SubtitulosRepo struct{ pool *pgxpool.Pool }

func NewSubtitulosRepo(pool *pgxpool.Pool) *SubtitulosRepo { return &SubtitulosRepo{pool: pool} }

// Subtitulo es una pista asociada a un recurso reproducible.
type Subtitulo struct {
	ID         uuid.UUID `json:"id"`
	ResourceID uuid.UUID `json:"resource_id"`
	Idioma     string    `json:"language"`
	Etiqueta   string    `json:"label"`
	ObjectKey  string    `json:"-"`
	// Transcripcion es el texto corrido derivado de la pista. No viaja en los
	// listados: en un vídeo largo son miles de palabras.
	Transcripcion string    `json:"transcript,omitempty"`
	Tipo          string    `json:"kind"`
	CreadoEn      time.Time `json:"created_at"`
}

// Guardar registra o reemplaza la pista de un idioma.
//
// Reemplaza en lugar de acumular: subir otra vez el mismo idioma es corregir
// la pista, no añadir una segunda que el reproductor no sabría cuál elegir.
func (r *SubtitulosRepo) Guardar(ctx context.Context, s *Subtitulo) error {
	return r.pool.QueryRow(ctx, `
		INSERT INTO resource_captions (resource_id, language, label, object_key, transcript, kind)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (resource_id, language) DO UPDATE SET
			label = EXCLUDED.label,
			object_key = EXCLUDED.object_key,
			transcript = EXCLUDED.transcript,
			kind = EXCLUDED.kind
		RETURNING id, created_at`,
		s.ResourceID, s.Idioma, s.Etiqueta, s.ObjectKey, s.Transcripcion, s.Tipo).
		Scan(&s.ID, &s.CreadoEn)
}

// Listar devuelve las pistas de un recurso, sin las transcripciones.
func (r *SubtitulosRepo) Listar(ctx context.Context, resourceID uuid.UUID) ([]Subtitulo, error) {
	filas, err := r.pool.Query(ctx, `
		SELECT id, resource_id, language, label, object_key, kind, created_at
		  FROM resource_captions WHERE resource_id = $1 ORDER BY language`, resourceID)
	if err != nil {
		return nil, err
	}
	defer filas.Close()

	out := []Subtitulo{}
	for filas.Next() {
		var s Subtitulo
		if err := filas.Scan(&s.ID, &s.ResourceID, &s.Idioma, &s.Etiqueta,
			&s.ObjectKey, &s.Tipo, &s.CreadoEn); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, filas.Err()
}

// PorIdioma devuelve una pista con su transcripción.
func (r *SubtitulosRepo) PorIdioma(ctx context.Context, resourceID uuid.UUID, idioma string) (*Subtitulo, error) {
	var s Subtitulo
	err := r.pool.QueryRow(ctx, `
		SELECT id, resource_id, language, label, object_key, transcript, kind, created_at
		  FROM resource_captions WHERE resource_id = $1 AND language = $2`, resourceID, idioma).
		Scan(&s.ID, &s.ResourceID, &s.Idioma, &s.Etiqueta, &s.ObjectKey,
			&s.Transcripcion, &s.Tipo, &s.CreadoEn)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return &s, err
}

func (r *SubtitulosRepo) Eliminar(ctx context.Context, resourceID uuid.UUID, idioma string) error {
	tag, err := r.pool.Exec(ctx,
		`DELETE FROM resource_captions WHERE resource_id = $1 AND language = $2`, resourceID, idioma)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
