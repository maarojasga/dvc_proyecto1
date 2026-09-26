package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// RevisionesRepo guarda el historial de un recurso de texto.
type RevisionesRepo struct{ pool *pgxpool.Pool }

func NewRevisionesRepo(pool *pgxpool.Pool) *RevisionesRepo { return &RevisionesRepo{pool: pool} }

// Revision es una versión guardada del contenido de un recurso.
type Revision struct {
	ID          uuid.UUID  `json:"id"`
	ResourceID  uuid.UUID  `json:"resource_id"`
	Numero      int        `json:"revision_number"`
	ContenidoMD string     `json:"content_md,omitempty"`
	AutorID     *uuid.UUID `json:"author_id,omitempty"`
	AutorEmail  string     `json:"author_email,omitempty"`
	CreadaEn    time.Time  `json:"created_at"`
}

// ErrSinCambios indica que el contenido es idéntico a la última revisión.
var ErrSinCambios = errors.New("revisiones: el contenido no cambió respecto a la última revisión")

// Guardar añade una revisión.
//
// Si el contenido es idéntico al de la última, no se guarda nada y se devuelve
// ErrSinCambios: el autoguardado dispara cada pocos segundos y, sin esta
// comprobación, media hora de escritura dejaría cientos de revisiones idénticas
// entre las pocas que de verdad interesan.
//
// El número se calcula dentro de la misma sentencia que inserta, no leyendo
// antes el máximo: con dos autores editando a la vez, leer y luego insertar
// deja una ventana en la que ambos calculan el mismo número y uno de los dos
// pierde su revisión contra la restricción de unicidad.
func (r *RevisionesRepo) Guardar(ctx context.Context, recursoID uuid.UUID, contenido string, autorID uuid.UUID, autorEmail string) (*Revision, error) {
	var ultima string
	err := r.pool.QueryRow(ctx, `
		SELECT content_md FROM resource_revisions
		 WHERE resource_id = $1 ORDER BY revision_number DESC LIMIT 1`, recursoID).Scan(&ultima)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return nil, err
	}
	if err == nil && ultima == contenido {
		return nil, ErrSinCambios
	}

	var rev Revision
	err = r.pool.QueryRow(ctx, `
		INSERT INTO resource_revisions (resource_id, revision_number, content_md, author_id, author_email)
		VALUES ($1,
		        coalesce((SELECT max(revision_number) FROM resource_revisions WHERE resource_id = $1), 0) + 1,
		        $2, $3, $4)
		RETURNING id, resource_id, revision_number, content_md, author_id, author_email, created_at`,
		recursoID, contenido, autorID, autorEmail).
		Scan(&rev.ID, &rev.ResourceID, &rev.Numero, &rev.ContenidoMD, &rev.AutorID, &rev.AutorEmail, &rev.CreadaEn)
	if err != nil {
		return nil, err
	}
	return &rev, nil
}

// Listar devuelve el historial, el más reciente primero.
//
// No incluye el contenido: un historial largo de un recurso extenso pesaría
// megabytes, y la lista solo necesita decir quién guardó y cuándo. El contenido
// se pide por revisión, al mirar una en concreto.
func (r *RevisionesRepo) Listar(ctx context.Context, recursoID uuid.UUID, limite int) ([]Revision, error) {
	if limite <= 0 || limite > 200 {
		limite = 50
	}
	filas, err := r.pool.Query(ctx, `
		SELECT id, resource_id, revision_number, author_id, coalesce(author_email, ''), created_at
		  FROM resource_revisions
		 WHERE resource_id = $1
		 ORDER BY revision_number DESC
		 LIMIT $2`, recursoID, limite)
	if err != nil {
		return nil, err
	}
	defer filas.Close()

	out := []Revision{}
	for filas.Next() {
		var rev Revision
		if err := filas.Scan(&rev.ID, &rev.ResourceID, &rev.Numero, &rev.AutorID, &rev.AutorEmail, &rev.CreadaEn); err != nil {
			return nil, err
		}
		out = append(out, rev)
	}
	return out, filas.Err()
}

// PorNumero devuelve una revisión concreta con su contenido.
func (r *RevisionesRepo) PorNumero(ctx context.Context, recursoID uuid.UUID, numero int) (*Revision, error) {
	var rev Revision
	err := r.pool.QueryRow(ctx, `
		SELECT id, resource_id, revision_number, content_md, author_id, coalesce(author_email, ''), created_at
		  FROM resource_revisions
		 WHERE resource_id = $1 AND revision_number = $2`, recursoID, numero).
		Scan(&rev.ID, &rev.ResourceID, &rev.Numero, &rev.ContenidoMD, &rev.AutorID, &rev.AutorEmail, &rev.CreadaEn)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return &rev, err
}
