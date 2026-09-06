package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type MediaAsset struct {
	ID                uuid.UUID
	ResourceID        uuid.UUID
	OriginalObjectKey string
	MimeType          string
	SizeBytes         int64
	ChecksumSHA256    string
	Status            string
	HLSMasterKey      string
	FailureReason     string
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

type MediaRepo struct{ pool *pgxpool.Pool }

func NewMediaRepo(pool *pgxpool.Pool) *MediaRepo { return &MediaRepo{pool: pool} }

func (r *MediaRepo) Create(ctx context.Context, a *MediaAsset) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO media_assets (id, resource_id, original_object_key, mime_type, size_bytes, checksum_sha256, status)
		VALUES ($1,$2,$3,$4,$5,$6,$7)`,
		a.ID, a.ResourceID, a.OriginalObjectKey, a.MimeType, a.SizeBytes, a.ChecksumSHA256, a.Status)
	return err
}

func (r *MediaRepo) GetByID(ctx context.Context, id uuid.UUID) (*MediaAsset, error) {
	var a MediaAsset
	err := r.pool.QueryRow(ctx, `
		SELECT id, resource_id, original_object_key, coalesce(mime_type,''), coalesce(size_bytes,0),
			coalesce(checksum_sha256,''), status, coalesce(hls_master_key,''), coalesce(failure_reason,''),
			created_at, updated_at
		FROM media_assets WHERE id=$1`, id).
		Scan(&a.ID, &a.ResourceID, &a.OriginalObjectKey, &a.MimeType, &a.SizeBytes, &a.ChecksumSHA256,
			&a.Status, &a.HLSMasterKey, &a.FailureReason, &a.CreatedAt, &a.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return &a, err
}

// LeaseProcesamiento es lo que dura el arrendamiento antes de que otro worker
// pueda reclamar un trabajo cuyo dueño no dio señales. Debe superar con
// holgura lo que tarda la transcodificación más larga que se espera.
const LeaseProcesamiento = 30 * time.Minute

// Reclamar intenta tomar el trabajo de transcodificación de forma exclusiva.
//
// Devuelve false, sin error, cuando el activo ya está listo o cuando otro
// worker lo está procesando con el arrendamiento vigente: en ambos casos este
// intento debe descartarse en silencio, que es lo que hace que una entrega
// duplicada no genere salidas repetidas.
//
// La condición vive en el propio UPDATE, así que dos entregas simultáneas se
// resuelven en la base y no queda ventana entre comprobar y actuar.
func (r *MediaRepo) Reclamar(ctx context.Context, id uuid.UUID, ahora time.Time) (bool, error) {
	tag, err := r.pool.Exec(ctx, `
		UPDATE media_assets
		   SET status = 'processing', processing_started_at = $2, updated_at = now()
		 WHERE id = $1
		   AND status <> 'ready'
		   AND (status <> 'processing'
		        OR processing_started_at IS NULL
		        OR processing_started_at < $3)`,
		id, ahora, ahora.Add(-LeaseProcesamiento))
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() == 1, nil
}

func (r *MediaRepo) MarkReady(ctx context.Context, id uuid.UUID, hlsMasterKey string) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE media_assets
		   SET status='ready', hls_master_key=$2, processing_started_at=NULL, updated_at=now()
		 WHERE id=$1`, id, hlsMasterKey)
	return err
}

func (r *MediaRepo) MarkFailed(ctx context.Context, id uuid.UUID, reason string) error {
	// Se libera el arrendamiento para que el reintento de asynq pueda
	// reclamarlo de inmediato, sin esperar a que venza.
	_, err := r.pool.Exec(ctx, `
		UPDATE media_assets
		   SET status='failed', failure_reason=$2, processing_started_at=NULL, updated_at=now()
		 WHERE id=$1`, id, reason)
	return err
}
