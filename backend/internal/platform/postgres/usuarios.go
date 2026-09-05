package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/DES-SOLUCIONES-CLOUD/proyecto-1/backend/internal/domain/user"
)

// RepositorioUsuarios implementa user.RepositorioUsuarios.
type RepositorioUsuarios struct{ pool *pgxpool.Pool }

// NuevoRepositorioUsuarios construye el repositorio.
func NuevoRepositorioUsuarios(pool *pgxpool.Pool) *RepositorioUsuarios {
	return &RepositorioUsuarios{pool: pool}
}

const codigoViolacionUnicidad = "23505"

// esUnicidadVioladaEn informa si el error es una violacion de UNIQUE sobre la
// restriccion indicada.
func esUnicidadVioladaEn(err error, restriccion string) bool {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return false
	}
	return pgErr.Code == codigoViolacionUnicidad && pgErr.ConstraintName == restriccion
}

// Crear inserta la cuenta y sus roles en una transaccion.
func (r *RepositorioUsuarios) Crear(ctx context.Context, u *user.Usuario) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(context.WithoutCancel(ctx)) }()

	_, err = tx.Exec(ctx, `
		INSERT INTO usuarios (id, correo, nombre, clave_hash, estado, correo_verificado_en, creado_en, actualizado_en)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		u.ID, u.Correo, u.Nombre, u.ClaveHash, string(u.Estado), u.CorreoVerificadoEn, u.CreadoEn, u.ActualizadoEn)
	if esUnicidadVioladaEn(err, "usuarios_correo_key") {
		return user.ErrCorreoRegistrado
	}
	if err != nil {
		return err
	}

	for _, rol := range u.Roles {
		if !rol.Valido() {
			return user.ErrRolNoValido
		}
		if _, err := tx.Exec(ctx,
			`INSERT INTO usuario_roles (usuario_id, rol) VALUES ($1, $2)`, u.ID, string(rol)); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

// consultaUsuario selecciona un usuario con sus roles agregados, para
// resolverlo en una sola ida a la base.
const consultaUsuario = `
	SELECT u.id, u.correo, u.nombre, u.clave_hash, u.estado, u.correo_verificado_en,
	       u.creado_en, u.actualizado_en,
	       COALESCE(ARRAY_AGG(r.rol) FILTER (WHERE r.rol IS NOT NULL), '{}') AS roles
	  FROM usuarios u
	  LEFT JOIN usuario_roles r ON r.usuario_id = u.id
	 WHERE %s
	 GROUP BY u.id`

func escanearUsuario(fila pgx.Row) (*user.Usuario, error) {
	var u user.Usuario
	var estado string
	var roles []string
	err := fila.Scan(&u.ID, &u.Correo, &u.Nombre, &u.ClaveHash, &estado,
		&u.CorreoVerificadoEn, &u.CreadoEn, &u.ActualizadoEn, &roles)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, user.ErrNoEncontrado
	}
	if err != nil {
		return nil, err
	}
	u.Estado = user.Estado(estado)
	u.Roles = make([]user.Rol, 0, len(roles))
	for _, rol := range roles {
		u.Roles = append(u.Roles, user.Rol(rol))
	}
	return &u, nil
}

// PorID devuelve el usuario con ese identificador.
func (r *RepositorioUsuarios) PorID(ctx context.Context, id uuid.UUID) (*user.Usuario, error) {
	return escanearUsuario(r.pool.QueryRow(ctx, formatearConsulta(consultaUsuario, "u.id = $1"), id))
}

// PorCorreo devuelve el usuario con ese correo ya normalizado.
func (r *RepositorioUsuarios) PorCorreo(ctx context.Context, correo string) (*user.Usuario, error) {
	return escanearUsuario(r.pool.QueryRow(ctx, formatearConsulta(consultaUsuario, "u.correo = $1"), correo))
}

// Activar marca el correo como verificado y pasa la cuenta a activa.
//
// Solo actua sobre cuentas pendientes: reusar un token no reactiva una cuenta
// que un administrador haya suspendido entretanto.
func (r *RepositorioUsuarios) Activar(ctx context.Context, id uuid.UUID, verificadoEn time.Time) error {
	etiqueta, err := r.pool.Exec(ctx, `
		UPDATE usuarios
		   SET estado = 'activo', correo_verificado_en = $2, actualizado_en = $2
		 WHERE id = $1 AND estado = 'pendiente'`, id, verificadoEn)
	if err != nil {
		return err
	}
	if etiqueta.RowsAffected() == 0 {
		return user.ErrNoEncontrado
	}
	return nil
}

// CambiarClave fija un nuevo hash de clave.
func (r *RepositorioUsuarios) CambiarClave(ctx context.Context, id uuid.UUID, claveHash string, ahora time.Time) error {
	etiqueta, err := r.pool.Exec(ctx, `
		UPDATE usuarios SET clave_hash = $2, actualizado_en = $3 WHERE id = $1`,
		id, claveHash, ahora)
	if err != nil {
		return err
	}
	if etiqueta.RowsAffected() == 0 {
		return user.ErrNoEncontrado
	}
	return nil
}
