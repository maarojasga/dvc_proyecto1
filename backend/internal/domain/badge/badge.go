// Package badge modela la emisión idempotente de insignias digitales
// verificables públicamente, sin exponer el correo del estudiante.
package badge

import (
	"crypto/rand"
	"encoding/base32"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
)

var ErrAlreadyIssued = errors.New("badge: ya existe una insignia emitida para esta inscripción")

// Badge es una insignia única emitida al aprobar un curso.
type Badge struct {
	ID               uuid.UUID
	EnrollmentID     uuid.UUID
	CourseID         uuid.UUID
	StudentID        uuid.UUID
	VerificationCode string
	ImageObjectKey   string
	IssuedAt         time.Time
	RevokedAt        *time.Time
	RevokedReason    string
}

// Verified indica si la insignia sigue siendo válida (no revocada).
func (b *Badge) Verified() bool {
	return b.RevokedAt == nil
}

// Revoke invalida la insignia dejando auditoría del motivo; la URL de
// verificación sigue existiendo pero reporta el estado revocado.
func (b *Badge) Revoke(reason string, now time.Time) {
	b.RevokedAt = &now
	b.RevokedReason = reason
}

// NewVerificationCode genera un código público aleatorio y no adivinable
// para la URL de verificación (no deriva del correo ni de datos personales).
func NewVerificationCode() (string, error) {
	buf := make([]byte, 20)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	code := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(buf)
	return strings.ToLower(code), nil
}

// New construye una insignia lista para persistir. El llamador debe
// garantizar idempotencia (una insignia por enrollment_id) a nivel de
// restricción única en base de datos.
func New(enrollmentID, courseID, studentID uuid.UUID, now time.Time) (*Badge, error) {
	code, err := NewVerificationCode()
	if err != nil {
		return nil, err
	}
	return &Badge{
		ID:               uuid.New(),
		EnrollmentID:     enrollmentID,
		CourseID:         courseID,
		StudentID:        studentID,
		VerificationCode: code,
		IssuedAt:         now,
	}, nil
}
