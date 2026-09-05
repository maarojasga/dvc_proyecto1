// Package postgres implementa los repositorios de dominio sobre PostgreSQL,
// fuente de verdad transaccional del sistema.
package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Abrir crea el pool de conexiones y comprueba que la base responde.
func Abrir(ctx context.Context, url string) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, fmt.Errorf("postgres: url invalida: %w", err)
	}
	// La API y los workers son sin estado y escalan horizontalmente: cada
	// instancia mantiene un pool acotado para no agotar max_connections.
	cfg.MaxConns = 10
	cfg.MaxConnLifetime = time.Hour
	cfg.MaxConnIdleTime = 5 * time.Minute

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("postgres: no se pudo crear el pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("postgres: sin conexion: %w", err)
	}
	return pool, nil
}
