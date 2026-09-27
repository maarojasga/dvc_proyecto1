// Package postgres provee el pool de conexión a PostgreSQL (fuente de
// verdad transaccional) y un runner de migraciones embebidas.
package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// conexionesPorDefecto es el pool de cada proceso cuando no se pide otro.
const conexionesPorDefecto = 20

// conexionesMaximas acota DB_MAX_CONNS. Ninguna instancia de esta entrega
// admite tantas; el tope existe para que la conversión a int32 no pueda
// desbordarse con un valor absurdo.
const conexionesMaximas = 1000

// Connect abre un pool de conexiones y verifica conectividad con un ping.
func Connect(ctx context.Context, databaseURL string) (*pgxpool.Pool, error) {
	return ConnectConMaximo(ctx, databaseURL, conexionesPorDefecto)
}

// ConnectConMaximo es Connect con un tamaño de pool explícito (DB_MAX_CONNS).
// Lo usan la API y el worker: sus pools se suman contra el max_connections de
// la base administrada, que en instancias pequeñas es de unas decenas.
func ConnectConMaximo(ctx context.Context, databaseURL string, maxConns int) (*pgxpool.Pool, error) {
	cfg, err := configDelPool(databaseURL, maxConns)
	if err != nil {
		return nil, err
	}

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("postgres: no se pudo crear el pool: %w", err)
	}

	pingCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("postgres: ping falló: %w", err)
	}
	return pool, nil
}

// configDelPool interpreta la URL y fija el tamaño del pool. Un máximo no
// positivo vuelve al valor por defecto en lugar de dejar un pool sin
// conexiones que colgaría la primera consulta.
func configDelPool(databaseURL string, maxConns int) (*pgxpool.Config, error) {
	cfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("postgres: config inválida: %w", err)
	}
	if maxConns <= 0 {
		maxConns = conexionesPorDefecto
	}
	if maxConns > conexionesMaximas {
		maxConns = conexionesMaximas
	}
	cfg.MaxConns = int32(maxConns)
	cfg.MaxConnLifetime = time.Hour
	return cfg, nil
}
