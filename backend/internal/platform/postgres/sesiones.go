package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/DES-SOLUCIONES-CLOUD/proyecto-1/backend/internal/domain/user"
)

// RepositorioSesiones implementa user.RepositorioSesiones.
type RepositorioSesiones struct{ pool *pgxpool.Pool }

// NuevoRepositorioSesiones construye el repositorio.
func NuevoRepositorioSesiones(pool *pgxpool.Pool) *RepositorioSesiones {
	return &RepositorioSesiones{pool: pool}
}

// Crear abre la sesion guardando solo el hash del token.
func (r *RepositorioSesiones) Crear(ctx context.Context, s user.Sesion, tokenHash []byte) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO sesiones (id, usuario_id, token_hash, creada_en, expira_en, ultimo_uso_en, ip, user_agent)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		s.ID, s.UsuarioID, tokenHash, s.CreadaEn, s.ExpiraEn, s.UltimoUsoEn, s.IP, s.UserAgent)
	return err
}

// PorTokenHashConUsuario resuelve la cookie en una sola consulta: sesion,
// titular y roles.
func (r *RepositorioSesiones) PorTokenHashConUsuario(ctx context.Context, tokenHash []byte) (user.Sesion, *user.Usuario, error) {
	fila := r.pool.QueryRow(ctx, `
		SELECT s.id, s.usuario_id, s.creada_en, s.expira_en, s.ultimo_uso_en,
		       s.revocada_en, s.ip, s.user_agent,
		       u.correo, u.nombre, u.clave_hash, u.estado, u.correo_verificado_en,
		       u.creado_en, u.actualizado_en,
		       COALESCE(ARRAY_AGG(r.rol) FILTER (WHERE r.rol IS NOT NULL), '{}') AS roles
		  FROM sesiones s
		  JOIN usuarios u ON u.id = s.usuario_id
		  LEFT JOIN usuario_roles r ON r.usuario_id = u.id
		 WHERE s.token_hash = $1
		 GROUP BY s.id, u.id`, tokenHash)

	var s user.Sesion
	var u user.Usuario
	var estado string
	var roles []string
	err := fila.Scan(&s.ID, &s.UsuarioID, &s.CreadaEn, &s.ExpiraEn, &s.UltimoUsoEn,
		&s.RevocadaEn, &s.IP, &s.UserAgent,
		&u.Correo, &u.Nombre, &u.ClaveHash, &estado, &u.CorreoVerificadoEn,
		&u.CreadoEn, &u.ActualizadoEn, &roles)
	if errors.Is(err, pgx.ErrNoRows) {
		return user.Sesion{}, nil, user.ErrNoEncontrado
	}
	if err != nil {
		return user.Sesion{}, nil, err
	}

	u.ID = s.UsuarioID
	u.Estado = user.Estado(estado)
	u.Roles = make([]user.Rol, 0, len(roles))
	for _, rol := range roles {
		u.Roles = append(u.Roles, user.Rol(rol))
	}
	return s, &u, nil
}

// ListarActivas devuelve las sesiones vigentes del usuario, la mas reciente
// primero.
func (r *RepositorioSesiones) ListarActivas(ctx context.Context, usuarioID uuid.UUID, ahora time.Time) ([]user.Sesion, error) {
	filas, err := r.pool.Query(ctx, `
		SELECT id, usuario_id, creada_en, expira_en, ultimo_uso_en, revocada_en, ip, user_agent
		  FROM sesiones
		 WHERE usuario_id = $1 AND revocada_en IS NULL AND expira_en > $2
		 ORDER BY ultimo_uso_en DESC`, usuarioID, ahora)
	if err != nil {
		return nil, err
	}
	defer filas.Close()

	var sesiones []user.Sesion
	for filas.Next() {
		var s user.Sesion
		if err := filas.Scan(&s.ID, &s.UsuarioID, &s.CreadaEn, &s.ExpiraEn,
			&s.UltimoUsoEn, &s.RevocadaEn, &s.IP, &s.UserAgent); err != nil {
			return nil, err
		}
		sesiones = append(sesiones, s)
	}
	return sesiones, filas.Err()
}

// Revocar cierra la sesion. Es idempotente: revocar dos veces no cambia la
// marca de tiempo original.
func (r *RepositorioSesiones) Revocar(ctx context.Context, id uuid.UUID, ahora time.Time) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE sesiones SET revocada_en = $2 WHERE id = $1 AND revocada_en IS NULL`, id, ahora)
	return err
}

// RevocarTodas cierra las sesiones activas del usuario y devuelve cuales,
// para que el llamador purgue la cache.
func (r *RepositorioSesiones) RevocarTodas(ctx context.Context, usuarioID uuid.UUID, excepto *uuid.UUID, ahora time.Time) ([]uuid.UUID, error) {
	filas, err := r.pool.Query(ctx, `
		UPDATE sesiones
		   SET revocada_en = $2
		 WHERE usuario_id = $1
		   AND revocada_en IS NULL
		   AND ($3::uuid IS NULL OR id <> $3)
		RETURNING id`, usuarioID, ahora, excepto)
	if err != nil {
		return nil, err
	}
	defer filas.Close()

	var revocadas []uuid.UUID
	for filas.Next() {
		var id uuid.UUID
		if err := filas.Scan(&id); err != nil {
			return nil, err
		}
		revocadas = append(revocadas, id)
	}
	return revocadas, filas.Err()
}

// TocarUltimoUso refresca la marca de actividad de la sesion.
func (r *RepositorioSesiones) TocarUltimoUso(ctx context.Context, id uuid.UUID, ahora time.Time) error {
	_, err := r.pool.Exec(ctx, `UPDATE sesiones SET ultimo_uso_en = $2 WHERE id = $1`, id, ahora)
	return err
}
