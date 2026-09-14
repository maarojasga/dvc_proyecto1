package postgres

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/DES-SOLUCIONES-CLOUD/proyecto-1/backend/internal/domain/iframe"
)

// IframeRepo persiste la lista blanca de destinos incrustables.
type IframeRepo struct{ pool *pgxpool.Pool }

func NewIframeRepo(pool *pgxpool.Pool) *IframeRepo { return &IframeRepo{pool: pool} }

const camposIframe = `id, host, include_subdomains, permissions, description`

// Listar devuelve la lista completa. Es corta por naturaleza —una entrada por
// proveedor autorizado— así que no se pagina.
func (r *IframeRepo) Listar(ctx context.Context) (iframe.Lista, error) {
	filas, err := r.pool.Query(ctx, `SELECT `+camposIframe+` FROM iframe_allowlist ORDER BY host`)
	if err != nil {
		return nil, err
	}
	defer filas.Close()

	var out iframe.Lista
	for filas.Next() {
		var d iframe.Destino
		if err := filas.Scan(&d.ID, &d.Host, &d.IncluirSubdominios, &d.Permisos, &d.Descripcion); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, filas.Err()
}

// Agregar inserta un destino. Un host repetido actualiza el existente en lugar
// de fallar: lo que quiere quien administra es que el host quede como acaba de
// describirlo.
func (r *IframeRepo) Agregar(ctx context.Context, d *iframe.Destino, actorID uuid.UUID) error {
	return r.pool.QueryRow(ctx, `
		INSERT INTO iframe_allowlist (host, include_subdomains, permissions, description, created_by)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (host) DO UPDATE SET
			include_subdomains = EXCLUDED.include_subdomains,
			permissions        = EXCLUDED.permissions,
			description        = EXCLUDED.description
		RETURNING id`,
		d.Host, d.IncluirSubdominios, d.Permisos, d.Descripcion, actorID).Scan(&d.ID)
}

func (r *IframeRepo) Eliminar(ctx context.Context, id uuid.UUID) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM iframe_allowlist WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *IframeRepo) PorID(ctx context.Context, id uuid.UUID) (*iframe.Destino, error) {
	var d iframe.Destino
	err := r.pool.QueryRow(ctx, `SELECT `+camposIframe+` FROM iframe_allowlist WHERE id = $1`, id).
		Scan(&d.ID, &d.Host, &d.IncluirSubdominios, &d.Permisos, &d.Descripcion)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return &d, err
}
