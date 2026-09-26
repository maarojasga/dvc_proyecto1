// Package user contiene las entidades y reglas de negocio de usuarios, roles
// (administrador, profesor, estudiante) y sesiones, sin depender de HTTP,
// base de datos ni proveedor cloud.
package user

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

// Role es el rol global del usuario dentro de la plataforma.
type Role string

const (
	RoleAdmin   Role = "admin"
	RoleTeacher Role = "teacher"
	RoleStudent Role = "student"
)

func (r Role) Valid() bool {
	switch r {
	case RoleAdmin, RoleTeacher, RoleStudent:
		return true
	}
	return false
}

// Status es el estado de la cuenta.
type Status string

const (
	StatusPendingVerification Status = "pending_verification"
	StatusActive              Status = "active"
	StatusSuspended           Status = "suspended"
)

var (
	ErrInvalidEmail         = errors.New("user: email inválido")
	ErrWeakPassword         = errors.New("user: la contraseña no cumple la política mínima")
	ErrAccountNotActive     = errors.New("user: la cuenta no está activa")
	ErrLastAdminProtected   = errors.New("user: no se puede degradar, suspender o eliminar al último administrador activo")
	ErrEmailAlreadyVerified = errors.New("user: el correo ya está verificado")
)

// User es la entidad de dominio de un usuario.
type User struct {
	ID              uuid.UUID
	Email           string
	PasswordHash    string
	FullName        string
	Role            Role
	Status          Status
	EmailVerifiedAt *time.Time
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// CanAuthenticate indica si la cuenta puede iniciar sesión.
func (u *User) CanAuthenticate() bool {
	return u.Status == StatusActive
}

// MarkEmailVerified activa la cuenta tras confirmar el correo.
func (u *User) MarkEmailVerified(now time.Time) error {
	if u.EmailVerifiedAt != nil {
		return ErrEmailAlreadyVerified
	}
	u.EmailVerifiedAt = &now
	u.Status = StatusActive
	u.UpdatedAt = now
	return nil
}

// IsAdmin, IsTeacher, IsStudent son atajos de comparación de rol.
func (u *User) IsAdmin() bool   { return u.Role == RoleAdmin }
func (u *User) IsTeacher() bool { return u.Role == RoleTeacher }
func (u *User) IsStudent() bool { return u.Role == RoleStudent }

// MinPasswordLength es la longitud mínima exigida a las contraseñas en texto
// plano antes de derivarlas con bcrypt.
const MinPasswordLength = 10

// ValidatePasswordStrength aplica la política mínima de contraseñas.
func ValidatePasswordStrength(plain string) error {
	if len(plain) < MinPasswordLength {
		return ErrWeakPassword
	}
	return nil
}
