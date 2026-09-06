package user

import (
	"time"

	"github.com/google/uuid"
)

// Session representa una sesión de usuario revocable. El token en texto
// plano nunca se persiste: solo se guarda su hash (TokenHash).
type Session struct {
	ID        uuid.UUID
	UserID    uuid.UUID
	TokenHash string
	UserAgent string
	IPAddress string
	CreatedAt time.Time
	ExpiresAt time.Time
	RevokedAt *time.Time
}

// Active indica si la sesión sigue siendo válida en el instante now.
func (s *Session) Active(now time.Time) bool {
	if s.RevokedAt != nil {
		return false
	}
	return now.Before(s.ExpiresAt)
}

// DefaultSessionTTL es la vigencia por defecto de una sesión.
const DefaultSessionTTL = 30 * 24 * time.Hour
