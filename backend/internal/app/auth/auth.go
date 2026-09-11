// Package auth implementa los casos de uso de identidad: registro con
// verificación de correo, sesiones revocables, recuperación de contraseña e
// invitación de profesores por administración.
package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/DES-SOLUCIONES-CLOUD/proyecto-1/backend/internal/domain/user"
	"github.com/DES-SOLUCIONES-CLOUD/proyecto-1/backend/internal/platform/mailer"
	"github.com/DES-SOLUCIONES-CLOUD/proyecto-1/backend/internal/platform/postgres"
	"github.com/DES-SOLUCIONES-CLOUD/proyecto-1/backend/internal/platform/security"
)

var (
	ErrEmailInUse            = errors.New("auth: el correo ya está registrado")
	ErrInvalidCredentials    = errors.New("auth: credenciales inválidas")
	ErrInvalidOrExpiredToken = errors.New("auth: token inválido o expirado")
	ErrSessionNotFound       = errors.New("auth: la sesión no existe o no es del usuario")
)

// decoyHash es un hash bcrypt válido de una contraseña que nadie conoce. Se
// usa para que verificar credenciales de un correo inexistente cueste lo
// mismo que verificar las de uno real.
var decoyHash = func() string {
	h, err := security.HashPassword(uuid.NewString())
	if err != nil {
		panic("auth: no se pudo derivar el hash señuelo: " + err.Error())
	}
	return h
}()

const (
	verificationTokenTTL = 24 * time.Hour
	passwordResetTTL     = 1 * time.Hour
)

type Service struct {
	users      *postgres.UserRepo
	mailer     *mailer.Mailer
	publicURL  string
	sessionTTL time.Duration
}

func NewService(users *postgres.UserRepo, m *mailer.Mailer, publicURL string, sessionTTL time.Duration) *Service {
	return &Service{users: users, mailer: m, publicURL: publicURL, sessionTTL: sessionTTL}
}

// RegisterStudent crea una cuenta de estudiante pendiente de verificación y
// envía el correo de confirmación. Los profesores solo se crean por
// administración (ver CreateTeacher).
func (s *Service) RegisterStudent(ctx context.Context, email, password, fullName string) error {
	if err := user.ValidatePasswordStrength(password); err != nil {
		return err
	}

	// El hash se calcula siempre, exista o no la cuenta, para que el tiempo de
	// respuesta no delate cuáles correos están registrados.
	hash, err := security.HashPassword(password)
	if err != nil {
		return err
	}

	if existing, err := s.users.GetByEmail(ctx, email); err == nil {
		// El correo ya existe. No se revela: la respuesta es idéntica a la de
		// un alta nueva. Si la cuenta sigue pendiente se reenvía el enlace,
		// porque de lo contrario un fallo de correo la dejaría bloqueada para
		// siempre: reintentar el alta caería una y otra vez en esta rama.
		s.audit(ctx, nil, "auth.register.duplicate_email", existing.ID.String(), "", nil)
		if existing.Status == user.StatusPendingVerification {
			return s.sendVerificationEmail(ctx, existing)
		}
		return nil
	} else if !errors.Is(err, postgres.ErrNotFound) {
		return err
	}

	now := time.Now().UTC()
	u := &user.User{
		ID:           uuid.New(),
		Email:        email,
		PasswordHash: hash,
		FullName:     fullName,
		Role:         user.RoleStudent,
		Status:       user.StatusPendingVerification,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	if err := s.users.Create(ctx, u); err != nil {
		// Carrera con otra alta simultánea del mismo correo: la restricción
		// UNIQUE de la tabla es el árbitro, no la consulta previa.
		if errors.Is(err, postgres.ErrDuplicateEmail) {
			return nil
		}
		return err
	}
	s.audit(ctx, &u.ID, "auth.register.created", u.ID.String(), "",
		map[string]any{"role": string(user.RoleStudent)})

	return s.sendVerificationEmail(ctx, u)
}

// ResendVerification reenvía el enlace de verificación. Responde sin error
// exista o no la cuenta, y no envía nada si ya está verificada.
func (s *Service) ResendVerification(ctx context.Context, email string) error {
	u, err := s.users.GetByEmail(ctx, email)
	switch {
	case errors.Is(err, postgres.ErrNotFound):
		return nil
	case err != nil:
		return err
	case u.Status != user.StatusPendingVerification:
		return nil
	}
	return s.sendVerificationEmail(ctx, u)
}

func (s *Service) sendVerificationEmail(ctx context.Context, u *user.User) error {
	plain, err := security.GenerateOpaqueToken()
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	err = s.users.CreateEmailVerificationToken(ctx, &postgres.Token{
		ID:        uuid.New(),
		UserID:    u.ID,
		TokenHash: security.HashToken(plain),
		ExpiresAt: now.Add(verificationTokenTTL),
		CreatedAt: now,
	})
	if err != nil {
		return err
	}
	link := fmt.Sprintf("%s/verificar-correo?token=%s", s.publicURL, plain)
	body := fmt.Sprintf("Hola %s,\n\nConfirma tu correo en la Plataforma MOOC visitando:\n%s\n\nEl enlace vence en 24 horas.", u.FullName, link)
	return s.mailer.Send(u.Email, "Confirma tu correo", body)
}

// VerifyEmail consume el token de verificación y activa la cuenta.
func (s *Service) VerifyEmail(ctx context.Context, token string) error {
	now := time.Now().UTC()
	userID, err := s.users.ConsumeEmailVerificationToken(ctx, security.HashToken(token), now)
	if err != nil {
		if errors.Is(err, postgres.ErrNotFound) {
			return ErrInvalidOrExpiredToken
		}
		return err
	}
	u, err := s.users.GetByID(ctx, userID)
	if err != nil {
		return err
	}
	if err := u.MarkEmailVerified(now); err != nil {
		return err
	}
	if err := s.users.Update(ctx, u); err != nil {
		return err
	}
	s.audit(ctx, &u.ID, "auth.email.verified", u.ID.String(), "", nil)
	return nil
}

// Login valida credenciales y crea una sesión revocable. Devuelve el token
// en texto plano (solo se persiste su hash).
func (s *Service) Login(ctx context.Context, email, password, userAgent, ip string) (token string, u *user.User, err error) {
	u, err = s.users.GetByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, postgres.ErrNotFound) {
			// Se compara igualmente contra un hash señuelo: sin esto, un
			// correo inexistente responde mucho antes que uno real, y esa
			// diferencia basta para enumerar cuentas.
			security.ComparePassword(decoyHash, password)
			s.audit(ctx, nil, "auth.login.failed", "", ip, map[string]any{"reason": "unknown_email"})
			return "", nil, ErrInvalidCredentials
		}
		return "", nil, err
	}
	if !security.ComparePassword(u.PasswordHash, password) {
		s.audit(ctx, &u.ID, "auth.login.failed", u.ID.String(), ip, map[string]any{"reason": "bad_password"})
		return "", nil, ErrInvalidCredentials
	}
	if !u.CanAuthenticate() {
		s.audit(ctx, &u.ID, "auth.login.failed", u.ID.String(), ip, map[string]any{"reason": string(u.Status)})
		return "", nil, user.ErrAccountNotActive
	}

	plain, err := security.GenerateOpaqueToken()
	if err != nil {
		return "", nil, err
	}
	now := time.Now().UTC()
	sess := &user.Session{
		ID:        uuid.New(),
		UserID:    u.ID,
		TokenHash: security.HashToken(plain),
		UserAgent: userAgent,
		IPAddress: ip,
		CreatedAt: now,
		ExpiresAt: now.Add(s.sessionTTL),
	}
	if err := s.users.CreateSession(ctx, sess); err != nil {
		return "", nil, err
	}
	s.audit(ctx, &u.ID, "auth.login.succeeded", sess.ID.String(), ip, nil)
	return plain, u, nil
}

// Authenticate resuelve un token de sesión en texto plano al usuario y a la
// sesión activa correspondientes; usado por el middleware HTTP.
func (s *Service) Authenticate(ctx context.Context, token string) (*user.User, *user.Session, error) {
	sess, err := s.users.GetSessionByTokenHash(ctx, security.HashToken(token))
	if err != nil {
		return nil, nil, err
	}
	if !sess.Active(time.Now().UTC()) {
		return nil, nil, ErrInvalidCredentials
	}
	u, err := s.users.GetByID(ctx, sess.UserID)
	if err != nil {
		return nil, nil, err
	}
	if !u.CanAuthenticate() {
		return nil, nil, user.ErrAccountNotActive
	}
	return u, sess, nil
}

// audit deja constancia de un evento de identidad. No devuelve error a
// propósito: perder una entrada de bitácora no debe tumbar la operación
// auditada, y el resto del flujo ya reporta sus propios fallos.
func (s *Service) audit(ctx context.Context, actorID *uuid.UUID, action, entityID, ip string, metadata map[string]any) {
	if metadata == nil {
		metadata = map[string]any{}
	}
	meta, err := json.Marshal(metadata)
	if err != nil {
		return
	}
	// El hecho auditado ya ocurrió: no debe cancelarse si el cliente corta.
	_ = s.users.InsertAudit(context.WithoutCancel(ctx), postgres.AuditEntry{
		ActorUserID: actorID,
		Action:      action,
		EntityType:  "user",
		EntityID:    entityID,
		Metadata:    meta,
		IPAddress:   ip,
	})
}

// ListSessions devuelve las sesiones activas del usuario.
func (s *Service) ListSessions(ctx context.Context, u *user.User) ([]*user.Session, error) {
	return s.users.ListActiveSessions(ctx, u.ID, time.Now().UTC())
}

// RevokeSession cierra una sesión propia del usuario.
//
// Una sesión ajena se comporta igual que una inexistente: la propiedad se
// comprueba dentro del UPDATE, así que el identificador de otra cuenta no se
// puede confirmar por la respuesta.
func (s *Service) RevokeSession(ctx context.Context, u *user.User, sessionID uuid.UUID) error {
	err := s.users.RevokeSessionOwnedBy(ctx, sessionID, u.ID, time.Now().UTC())
	if errors.Is(err, postgres.ErrNotFound) {
		return ErrSessionNotFound
	}
	if err == nil {
		s.audit(ctx, &u.ID, "auth.session.revoked", sessionID.String(), "", nil)
	}
	return err
}

// RevokeOtherSessions cierra todas las sesiones del usuario salvo la actual,
// y devuelve cuántas cerró.
func (s *Service) RevokeOtherSessions(ctx context.Context, u *user.User, current *user.Session) (int, error) {
	revoked, err := s.users.RevokeOtherSessions(ctx, u.ID, current.ID, time.Now().UTC())
	if err == nil {
		s.audit(ctx, &u.ID, "auth.sessions.revoked_others", u.ID.String(), "",
			map[string]any{"revoked": revoked})
	}
	return revoked, err
}

// Logout revoca la sesión asociada al token entregado (revocación
// inmediata).
func (s *Service) Logout(ctx context.Context, token string) error {
	sess, err := s.users.GetSessionByTokenHash(ctx, security.HashToken(token))
	if err != nil {
		if errors.Is(err, postgres.ErrNotFound) {
			return nil
		}
		return err
	}
	if err := s.users.RevokeSession(ctx, sess.ID, time.Now().UTC()); err != nil {
		return err
	}
	s.audit(ctx, &sess.UserID, "auth.logout", sess.ID.String(), "", nil)
	return nil
}

// RequestPasswordReset siempre responde sin error (evita enumeración de
// usuarios) pero solo envía correo si la cuenta existe.
func (s *Service) RequestPasswordReset(ctx context.Context, email string) error {
	u, err := s.users.GetByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, postgres.ErrNotFound) {
			return nil
		}
		return err
	}
	plain, err := security.GenerateOpaqueToken()
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	if err := s.users.CreatePasswordResetToken(ctx, &postgres.Token{
		ID:        uuid.New(),
		UserID:    u.ID,
		TokenHash: security.HashToken(plain),
		ExpiresAt: now.Add(passwordResetTTL),
		CreatedAt: now,
	}); err != nil {
		return err
	}
	link := fmt.Sprintf("%s/restablecer-contrasena?token=%s", s.publicURL, plain)
	body := fmt.Sprintf("Hola %s,\n\nRestablece tu contraseña visitando:\n%s\n\nEl enlace vence en 1 hora. Si no lo solicitaste, ignora este correo.", u.FullName, link)
	return s.mailer.Send(u.Email, "Restablecer contraseña", body)
}

// ConfirmPasswordReset consume el token y establece la nueva contraseña,
// revocando todas las sesiones activas por seguridad.
func (s *Service) ConfirmPasswordReset(ctx context.Context, token, newPassword string) error {
	if err := user.ValidatePasswordStrength(newPassword); err != nil {
		return err
	}
	now := time.Now().UTC()
	userID, err := s.users.ConsumePasswordResetToken(ctx, security.HashToken(token), now)
	if err != nil {
		if errors.Is(err, postgres.ErrNotFound) {
			return ErrInvalidOrExpiredToken
		}
		return err
	}
	u, err := s.users.GetByID(ctx, userID)
	if err != nil {
		return err
	}
	hash, err := security.HashPassword(newPassword)
	if err != nil {
		return err
	}
	u.PasswordHash = hash
	u.UpdatedAt = now
	if err := s.users.Update(ctx, u); err != nil {
		return err
	}
	// Quien recupera la clave puede estar expulsando a un intruso: se cierran
	// todas las sesiones, no solo las del navegador que hizo el cambio.
	if err := s.users.RevokeAllSessionsForUser(ctx, u.ID, now); err != nil {
		return err
	}
	s.audit(ctx, &u.ID, "auth.password.reset_confirmed", u.ID.String(), "", nil)
	return nil
}

// CreateTeacher crea una cuenta de profesor ya activa (los profesores no se
// autorregistran) y envía un enlace de establecimiento de contraseña.
func (s *Service) CreateTeacher(ctx context.Context, email, fullName string) (*user.User, error) {
	if _, err := s.users.GetByEmail(ctx, email); err == nil {
		return nil, ErrEmailInUse
	} else if !errors.Is(err, postgres.ErrNotFound) {
		return nil, err
	}

	randomPassword, err := security.GenerateOpaqueToken()
	if err != nil {
		return nil, err
	}
	hash, err := security.HashPassword(randomPassword)
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	u := &user.User{
		ID:              uuid.New(),
		Email:           email,
		PasswordHash:    hash,
		FullName:        fullName,
		Role:            user.RoleTeacher,
		Status:          user.StatusActive,
		EmailVerifiedAt: &now,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	if err := s.users.Create(ctx, u); err != nil {
		return nil, err
	}

	plain, err := security.GenerateOpaqueToken()
	if err != nil {
		return u, err
	}
	if err := s.users.CreatePasswordResetToken(ctx, &postgres.Token{
		ID:        uuid.New(),
		UserID:    u.ID,
		TokenHash: security.HashToken(plain),
		ExpiresAt: now.Add(48 * time.Hour),
		CreatedAt: now,
	}); err != nil {
		return u, err
	}
	link := fmt.Sprintf("%s/restablecer-contrasena?token=%s", s.publicURL, plain)
	body := fmt.Sprintf("Hola %s,\n\nSe creó una cuenta de profesor para ti en la Plataforma MOOC.\nEstablece tu contraseña en:\n%s\n\nEl enlace vence en 48 horas.", u.FullName, link)
	if err := s.mailer.Send(u.Email, "Bienvenido: establece tu contraseña", body); err != nil {
		return u, err
	}
	return u, nil
}
