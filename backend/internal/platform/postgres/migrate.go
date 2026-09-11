package postgres

import (
	"context"
	"fmt"
	"io/fs"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

const migrationsTable = `
CREATE TABLE IF NOT EXISTS schema_migrations (
    version     TEXT PRIMARY KEY,
    applied_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);`

// Migrate aplica, en orden y dentro de una transacción por archivo, las
// migraciones *.up.sql embebidas que aún no estén registradas en
// schema_migrations. Es seguro llamarla en cada arranque de la API.
func Migrate(ctx context.Context, pool *pgxpool.Pool, migrationsFS fs.FS) error {
	// La API y los workers escalan a varias instancias y todos migran al
	// arrancar. Sin exclusión mutua, dos arranques simultáneos leen
	// schema_migrations vacía y aplican la misma migración dos veces. El
	// advisory lock la serializa: el segundo espera y luego no ve nada
	// pendiente.
	conn, err := pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("migrate: no se pudo tomar una conexión: %w", err)
	}
	defer conn.Release()

	const migrationLockKey = 8274123409
	if _, err := conn.Exec(ctx, `SELECT pg_advisory_lock($1)`, migrationLockKey); err != nil {
		return fmt.Errorf("migrate: no se pudo tomar el lock: %w", err)
	}
	defer func() {
		// Sin cancelación: soltar el lock debe ocurrir aunque el contexto de
		// arranque ya haya expirado, o quedaría retenido hasta cerrar la
		// conexión.
		_, _ = conn.Exec(context.WithoutCancel(ctx), `SELECT pg_advisory_unlock($1)`, migrationLockKey)
	}()

	if _, err := pool.Exec(ctx, migrationsTable); err != nil {
		return fmt.Errorf("migrate: no se pudo crear schema_migrations: %w", err)
	}

	entries, err := fs.ReadDir(migrationsFS, ".")
	if err != nil {
		return fmt.Errorf("migrate: no se pudo leer el directorio de migraciones: %w", err)
	}

	var versions []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".up.sql") {
			continue
		}
		versions = append(versions, e.Name())
	}
	sort.Strings(versions)

	applied, err := appliedVersions(ctx, pool)
	if err != nil {
		return err
	}

	for _, name := range versions {
		version := strings.TrimSuffix(name, ".up.sql")
		if applied[version] {
			continue
		}

		content, err := fs.ReadFile(migrationsFS, name)
		if err != nil {
			return fmt.Errorf("migrate: no se pudo leer %s: %w", name, err)
		}

		tx, err := pool.Begin(ctx)
		if err != nil {
			return fmt.Errorf("migrate: no se pudo iniciar transacción para %s: %w", name, err)
		}

		if _, err := tx.Exec(ctx, string(content)); err != nil {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("migrate: falló %s: %w", name, err)
		}
		if _, err := tx.Exec(ctx, `INSERT INTO schema_migrations (version) VALUES ($1)`, version); err != nil {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("migrate: no se pudo registrar %s: %w", name, err)
		}
		if err := tx.Commit(ctx); err != nil {
			return fmt.Errorf("migrate: no se pudo confirmar %s: %w", name, err)
		}
	}
	return nil
}

func appliedVersions(ctx context.Context, pool *pgxpool.Pool) (map[string]bool, error) {
	rows, err := pool.Query(ctx, `SELECT version FROM schema_migrations`)
	if err != nil {
		return nil, fmt.Errorf("migrate: no se pudo consultar versiones aplicadas: %w", err)
	}
	defer rows.Close()

	applied := make(map[string]bool)
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		applied[v] = true
	}
	return applied, rows.Err()
}
