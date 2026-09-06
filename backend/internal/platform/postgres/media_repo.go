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
	ID                 uuid.UUID
	ResourceID         uuid.UUID
	OriginalObjectKey  string
	MimeType           string
	SizeBytes          int64
	ChecksumSHA256     string
	Status             string
	HLSMasterKey       string
	FailureReason      string
	CreatedAt          time.Time
	UpdatedAt          time.Time
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

func (r *MediaRepo) MarkProcessing(ctx context.Context, id uuid.UUID) error {
	_, err := r.pool.Exec(ctx, `UPDATE media_assets SET status='processing', updated_at=now() WHERE id=$1`, id)
	return err
}

func (r *MediaRepo) MarkReady(ctx context.Context, id uuid.UUID, hlsMasterKey string) error {
	_, err := r.pool.Exec(ctx, `UPDATE media_assets SET status='ready', hls_master_key=$2, updated_at=now() WHERE id=$1`, id, hlsMasterKey)
	return err
}

func (r *MediaRepo) MarkFailed(ctx context.Context, id uuid.UUID, reason string) error {
	_, err := r.pool.Exec(ctx, `UPDATE media_assets SET status='failed', failure_reason=$2, updated_at=now() WHERE id=$1`, id, reason)
	return err
}
