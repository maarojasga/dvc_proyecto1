package postgres

import (
	"context"
	"fmt"
	"io/fs"
	"log/slog"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/DES-SOLUCIONES-CLOUD/proyecto-1/backend/migrations"
)

// Migrar aplica en orden las migraciones pendientes, cada una en su propia
// transaccion. Es idempotente: relanzar la API no repite lo ya aplicado.
//
// Se toma un lock de sesion de PostgreSQL para que arrancar varias instancias
// a la vez no las aplique dos veces.
func Migrar(ctx context.Context, pool *pgxpool.Pool, log *slog.Logger) error {
	conn, err := pool.Acquire(ctx)
	if err != nil {
		return err
	}
	defer conn.Release()

	const claveLock = 8274123409 // arbitraria, exclusiva de este migrador
	if _, err := conn.Exec(ctx, "SELECT pg_advisory_lock($1)", claveLock); err != nil {
		return fmt.Errorf("postgres: no se pudo tomar el lock de migracion: %w", err)
	}
	defer func() {
		_, _ = conn.Exec(context.WithoutCancel(ctx), "SELECT pg_advisory_unlock($1)", claveLock)
	}()

	if _, err := conn.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version    text        PRIMARY KEY,
			aplicada_en timestamptz NOT NULL DEFAULT now()
		)`); err != nil {
		return fmt.Errorf("postgres: no se pudo crear schema_migrations: %w", err)
	}

	aplicadas := map[string]bool{}
	filas, err := conn.Query(ctx, "SELECT version FROM schema_migrations")
	if err != nil {
		return err
	}
	for filas.Next() {
		var v string
		if err := filas.Scan(&v); err != nil {
			filas.Close()
			return err
		}
		aplicadas[v] = true
	}
	filas.Close()
	if err := filas.Err(); err != nil {
		return err
	}

	versiones, err := versionesDisponibles()
	if err != nil {
		return err
	}

	for _, v := range versiones {
		if aplicadas[v.version] {
			continue
		}
		sql, err := fs.ReadFile(migrations.Archivos, v.ruta)
		if err != nil {
			return err
		}
		tx, err := conn.Begin(ctx)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, string(sql)); err != nil {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("postgres: fallo la migracion %s: %w", v.version, err)
		}
		if _, err := tx.Exec(ctx, "INSERT INTO schema_migrations (version) VALUES ($1)", v.version); err != nil {
			_ = tx.Rollback(ctx)
			return err
		}
		if err := tx.Commit(ctx); err != nil {
			return err
		}
		if log != nil {
			log.Info("migracion aplicada", "version", v.version)
		}
	}
	return nil
}

type migracion struct{ version, ruta string }

// versionesDisponibles lista las migraciones .up.sql ordenadas por version.
func versionesDisponibles() ([]migracion, error) {
	entradas, err := fs.ReadDir(migrations.Archivos, ".")
	if err != nil {
		return nil, err
	}
	var ms []migracion
	for _, e := range entradas {
		nombre := e.Name()
		if !strings.HasSuffix(nombre, ".up.sql") {
			continue
		}
		ms = append(ms, migracion{
			version: strings.TrimSuffix(nombre, ".up.sql"),
			ruta:    nombre,
		})
	}
	sort.Slice(ms, func(i, j int) bool { return ms[i].version < ms[j].version })
	return ms, nil
}
