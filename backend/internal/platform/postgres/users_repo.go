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

var (
	ErrNotFound = errors.New("postgres: registro no encontrado")
	// ErrDuplicateEmail traduce la violación del UNIQUE de users.email, que
	// es el árbitro real frente a dos altas simultáneas del mismo correo.
	ErrDuplicateEmail = errors.New("postgres: el correo ya está registrado")
)

type UserRepo struct{ pool *pgxpool.Pool }

func NewUserRepo(pool *pgxpool.Pool) *UserRepo { return &UserRepo{pool: pool} }

func scanUser(row pgx.Row) (*user.User, error) {
	var u user.User
	if err := row.Scan(&u.ID, &u.Email, &u.PasswordHash, &u.FullName, &u.Role, &u.Status,
		&u.EmailVerifiedAt, &u.CreatedAt, &u.UpdatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &u, nil
}

const userColumns = `id, email, password_hash, full_name, role, status, email_verified_at, created_at, updated_at`

func (r *UserRepo) Create(ctx context.Context, u *user.User) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO users (id, email, password_hash, full_name, role, status, email_verified_at, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`,
		u.ID, u.Email, u.PasswordHash, u.FullName, u.Role, u.Status, u.EmailVerifiedAt, u.CreatedAt, u.UpdatedAt)
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return ErrDuplicateEmail
	}
	return err
}

func (r *UserRepo) GetByID(ctx context.Context, id uuid.UUID) (*user.User, error) {
	row := r.pool.QueryRow(ctx, `SELECT `+userColumns+` FROM users WHERE id = $1`, id)
	return scanUser(row)
}

func (r *UserRepo) GetByEmail(ctx context.Context, email string) (*user.User, error) {
	row := r.pool.QueryRow(ctx, `SELECT `+userColumns+` FROM users WHERE email = $1`, email)
	return scanUser(row)
}

func (r *UserRepo) Update(ctx context.Context, u *user.User) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE users SET email=$2, password_hash=$3, full_name=$4, role=$5, status=$6,
			email_verified_at=$7, updated_at=$8
		WHERE id=$1`,
		u.ID, u.Email, u.PasswordHash, u.FullName, u.Role, u.Status, u.EmailVerifiedAt, u.UpdatedAt)
	return err
}

func (r *UserRepo) CountActiveAdmins(ctx context.Context) (int, error) {
	var n int
	err := r.pool.QueryRow(ctx, `SELECT count(*) FROM users WHERE role='admin' AND status='active'`).Scan(&n)
	return n, err
}

type ListUsersFilter struct {
	Role   string
	Status string
	Search string
	Limit  int
	Offset int
}

func (r *UserRepo) List(ctx context.Context, f ListUsersFilter) ([]*user.User, error) {
	limit := f.Limit
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	rows, err := r.pool.Query(ctx, `
		SELECT `+userColumns+` FROM users
		WHERE ($1 = '' OR role = $1)
		  AND ($2 = '' OR status = $2)
		  AND ($3 = '' OR email ILIKE '%' || $3 || '%' OR full_name ILIKE '%' || $3 || '%')
		ORDER BY created_at DESC
		LIMIT $4 OFFSET $5`,
		f.Role, f.Status, f.Search, limit, f.Offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*user.User
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

// --- Tokens de verificación de correo y reseteo de contraseña ---

type Token struct {
	ID        uuid.UUID
	UserID    uuid.UUID
	TokenHash string
	ExpiresAt time.Time
	UsedAt    *time.Time
	CreatedAt time.Time
}

func (r *UserRepo) CreateEmailVerificationToken(ctx context.Context, t *Token) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO email_verification_tokens (id, user_id, token_hash, expires_at, created_at)
		VALUES ($1,$2,$3,$4,$5)`, t.ID, t.UserID, t.TokenHash, t.ExpiresAt, t.CreatedAt)
	return err
}

func (r *UserRepo) ConsumeEmailVerificationToken(ctx context.Context, tokenHash string, now time.Time) (uuid.UUID, error) {
	var userID uuid.UUID
	err := r.pool.QueryRow(ctx, `
		UPDATE email_verification_tokens SET used_at=$2
		WHERE token_hash=$1 AND used_at IS NULL AND expires_at > $2
		RETURNING user_id`, tokenHash, now).Scan(&userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, ErrNotFound
	}
	return userID, err
}

func (r *UserRepo) CreatePasswordResetToken(ctx context.Context, t *Token) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO password_reset_tokens (id, user_id, token_hash, expires_at, created_at)
		VALUES ($1,$2,$3,$4,$5)`, t.ID, t.UserID, t.TokenHash, t.ExpiresAt, t.CreatedAt)
	return err
}

func (r *UserRepo) ConsumePasswordResetToken(ctx context.Context, tokenHash string, now time.Time) (uuid.UUID, error) {
	var userID uuid.UUID
	err := r.pool.QueryRow(ctx, `
		UPDATE password_reset_tokens SET used_at=$2
		WHERE token_hash=$1 AND used_at IS NULL AND expires_at > $2
		RETURNING user_id`, tokenHash, now).Scan(&userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, ErrNotFound
	}
	return userID, err
}

// --- Sesiones ---

func (r *UserRepo) CreateSession(ctx context.Context, s *user.Session) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO sessions (id, user_id, token_hash, user_agent, ip_address, created_at, expires_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7)`,
		s.ID, s.UserID, s.TokenHash, s.UserAgent, s.IPAddress, s.CreatedAt, s.ExpiresAt)
	return err
}

func (r *UserRepo) GetSessionByTokenHash(ctx context.Context, tokenHash string) (*user.Session, error) {
	var s user.Session
	err := r.pool.QueryRow(ctx, `
		SELECT id, user_id, token_hash, user_agent, ip_address, created_at, expires_at, revoked_at
		FROM sessions WHERE token_hash=$1`, tokenHash).
		Scan(&s.ID, &s.UserID, &s.TokenHash, &s.UserAgent, &s.IPAddress, &s.CreatedAt, &s.ExpiresAt, &s.RevokedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return &s, err
}

func (r *UserRepo) RevokeSession(ctx context.Context, id uuid.UUID, now time.Time) error {
	_, err := r.pool.Exec(ctx, `UPDATE sessions SET revoked_at=$2 WHERE id=$1 AND revoked_at IS NULL`, id, now)
	return err
}

// RevokeAllSessionsForUser invalida todas las sesiones de un usuario, usado
// al suspender la cuenta o al cambiar la contraseña.
func (r *UserRepo) RevokeAllSessionsForUser(ctx context.Context, userID uuid.UUID, now time.Time) error {
	_, err := r.pool.Exec(ctx, `UPDATE sessions SET revoked_at=$2 WHERE user_id=$1 AND revoked_at IS NULL`, userID, now)
	return err
}

// ListActiveSessions devuelve las sesiones vigentes del usuario, la de uso
// más reciente primero, para que pueda revisarlas y revocarlas.
func (r *UserRepo) ListActiveSessions(ctx context.Context, userID uuid.UUID, now time.Time) ([]*user.Session, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, user_id, token_hash, user_agent, ip_address, created_at, expires_at, revoked_at
		FROM sessions
		WHERE user_id=$1 AND revoked_at IS NULL AND expires_at > $2
		ORDER BY created_at DESC`, userID, now)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sessions []*user.Session
	for rows.Next() {
		var s user.Session
		if err := rows.Scan(&s.ID, &s.UserID, &s.TokenHash, &s.UserAgent, &s.IPAddress,
			&s.CreatedAt, &s.ExpiresAt, &s.RevokedAt); err != nil {
			return nil, err
		}
		sessions = append(sessions, &s)
	}
	return sessions, rows.Err()
}

// RevokeSessionOwnedBy revoca una sesión solo si pertenece al usuario
// indicado. La condición de propiedad va en el propio UPDATE para que no
// exista ventana entre comprobar y revocar; si no afecta ninguna fila el
// llamador no puede distinguir una sesión ajena de una inexistente.
func (r *UserRepo) RevokeSessionOwnedBy(ctx context.Context, id, userID uuid.UUID, now time.Time) error {
	tag, err := r.pool.Exec(ctx, `
		UPDATE sessions SET revoked_at=$3
		WHERE id=$1 AND user_id=$2 AND revoked_at IS NULL`, id, userID, now)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// RevokeOtherSessions revoca las sesiones activas del usuario salvo la
// indicada, y devuelve cuántas cerró.
func (r *UserRepo) RevokeOtherSessions(ctx context.Context, userID, keepID uuid.UUID, now time.Time) (int, error) {
	tag, err := r.pool.Exec(ctx, `
		UPDATE sessions SET revoked_at=$3
		WHERE user_id=$1 AND id<>$2 AND revoked_at IS NULL`, userID, keepID, now)
	if err != nil {
		return 0, err
	}
	return int(tag.RowsAffected()), nil
}

// --- Auditoría ---

type AuditEntry struct {
	ActorUserID *uuid.UUID
	Action      string
	EntityType  string
	EntityID    string
	Metadata    []byte // JSON
	IPAddress   string
}

func (r *UserRepo) InsertAudit(ctx context.Context, e AuditEntry) error {
	meta := e.Metadata
	if meta == nil {
		meta = []byte(`{}`)
	}
	_, err := r.pool.Exec(ctx, `
		INSERT INTO audit_logs (actor_user_id, action, entity_type, entity_id, metadata, ip_address)
		VALUES ($1,$2,$3,$4,$5,$6)`,
		e.ActorUserID, e.Action, e.EntityType, e.EntityID, meta, e.IPAddress)
	return err
}
