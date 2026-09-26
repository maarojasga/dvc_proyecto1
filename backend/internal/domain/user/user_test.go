package user

import (
	"testing"
	"time"
)

func TestValidatePasswordStrength(t *testing.T) {
	if err := ValidatePasswordStrength("corta"); err != ErrWeakPassword {
		t.Fatalf("esperaba ErrWeakPassword para contraseña corta, obtuve %v", err)
	}
	if err := ValidatePasswordStrength("contraseñaSegura123"); err != nil {
		t.Fatalf("no se esperaba error: %v", err)
	}
}

func TestMarkEmailVerified(t *testing.T) {
	u := &User{Status: StatusPendingVerification}
	now := time.Now().UTC()

	if err := u.MarkEmailVerified(now); err != nil {
		t.Fatalf("no se esperaba error: %v", err)
	}
	if u.Status != StatusActive {
		t.Fatalf("esperaba estado active, obtuve %s", u.Status)
	}
	if !u.CanAuthenticate() {
		t.Fatal("esperaba que la cuenta pudiera autenticarse tras verificar el correo")
	}

	if err := u.MarkEmailVerified(now); err != ErrEmailAlreadyVerified {
		t.Fatalf("esperaba ErrEmailAlreadyVerified al reverificar, obtuve %v", err)
	}
}

func TestSessionActive(t *testing.T) {
	now := time.Now().UTC()
	s := &Session{ExpiresAt: now.Add(time.Hour)}
	if !s.Active(now) {
		t.Fatal("esperaba sesión activa antes de expirar")
	}

	expired := &Session{ExpiresAt: now.Add(-time.Hour)}
	if expired.Active(now) {
		t.Fatal("esperaba sesión inactiva tras expirar")
	}

	revokedAt := now.Add(-time.Minute)
	revoked := &Session{ExpiresAt: now.Add(time.Hour), RevokedAt: &revokedAt}
	if revoked.Active(now) {
		t.Fatal("esperaba sesión inactiva tras revocación, incluso sin expirar")
	}
}
