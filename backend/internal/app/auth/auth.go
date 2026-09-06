// Package auth implementa los casos de uso de identidad: registro con
// verificación de correo, sesiones revocables, recuperación de contraseña e
// invitación de profesores por administración.
package auth

import (
	"context"
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
	ErrEmailInUse         = errors.New("auth: el correo ya está registrado")
	ErrInvalidCredentials = errors.New("auth: credenciales inválidas")
	ErrInvalidOrExpiredToken = errors.New("auth: token inválido o expirado")
)

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
func (s *Service) RegisterStudent(ctx context.Context, email, password, fullName string) (*user.User, error) {
	if err := user.ValidatePasswordStrength(password); err != nil {
		return nil, err
	}
	if _, err := s.users.GetByEmail(ctx, email); err == nil {
		return nil, ErrEmailInUse
	} else if !errors.Is(err, postgres.ErrNotFound) {
		return nil, err
	}

	hash, err := security.HashPassword(password)
	if err != nil {
		return nil, err
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
		return nil, err
	}

	if err := s.sendVerificationEmail(ctx, u); err != nil {
		return u, fmt.Errorf("auth: usuario creado pero falló el envío de verificación: %w", err)
	}
	return u, nil
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
	return s.users.Update(ctx, u)
}

// Login valida credenciales y crea una sesión revocable. Devuelve el token
// en texto plano (solo se persiste su hash).
func (s *Service) Login(ctx context.Context, email, password, userAgent, ip string) (token string, u *user.User, err error) {
	u, err = s.users.GetByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, postgres.ErrNotFound) {
			return "", nil, ErrInvalidCredentials
		}
		return "", nil, err
	}
	if !security.ComparePassword(u.PasswordHash, password) {
		return "", nil, ErrInvalidCredentials
	}
	if !u.CanAuthenticate() {
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
	return s.users.RevokeSession(ctx, sess.ID, time.Now().UTC())
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
	return s.users.RevokeAllSessionsForUser(ctx, u.ID, now)
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
