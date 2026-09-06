package badge

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestNew_GeneraCodigoDeVerificacionSinDatosPersonales(t *testing.T) {
	b, err := New(uuid.New(), uuid.New(), uuid.New(), time.Now())
	if err != nil {
		t.Fatalf("no se esperaba error: %v", err)
	}
	if b.VerificationCode == "" {
		t.Fatal("esperaba un código de verificación no vacío")
	}
	if strings.Contains(b.VerificationCode, "@") {
		t.Fatal("el código de verificación no debe contener datos con forma de correo")
	}
	if !b.Verified() {
		t.Fatal("una insignia recién emitida debe estar verificada")
	}
}

func TestRevoke_InvalidaLaInsignia(t *testing.T) {
	b, _ := New(uuid.New(), uuid.New(), uuid.New(), time.Now())
	b.Revoke("fraude detectado", time.Now())

	if b.Verified() {
		t.Fatal("esperaba que la insignia revocada no esté verificada")
	}
	if b.RevokedReason == "" {
		t.Fatal("esperaba auditar el motivo de revocación")
	}
}

func TestNewVerificationCode_EsUnico(t *testing.T) {
	a, _ := NewVerificationCode()
	b, _ := NewVerificationCode()
	if a == b {
		t.Fatal("esperaba códigos de verificación distintos entre llamadas")
	}
}
