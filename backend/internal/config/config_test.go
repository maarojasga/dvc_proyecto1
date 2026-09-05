package config

import "testing"

func TestSMTPVacioDesactivaElEnvio(t *testing.T) {
	// Fijar SMTP_ADDR a la cadena vacia es una decision del operador: pide
	// que no haya servidor de correo. Tratarlo como "sin definir" haria lo
	// contrario, apuntando al valor por defecto.
	t.Setenv("SMTP_ADDR", "")
	if got := Load().SMTPDireccion; got != "" {
		t.Errorf("SMTPDireccion = %q; se esperaba vacia", got)
	}
}

func TestSMTPAusenteUsaElValorPorDefecto(t *testing.T) {
	if got := Load().SMTPDireccion; got != "localhost:1025" {
		t.Errorf("SMTPDireccion = %q; se esperaba el valor por defecto", got)
	}
}

func TestCookieSeguraSegunEntorno(t *testing.T) {
	t.Setenv("APP_ENV", "development")
	if Load().CookieSegura {
		t.Error("en desarrollo, sin TLS, la cookie no puede exigir Secure")
	}
	t.Setenv("APP_ENV", "production")
	if !Load().CookieSegura {
		t.Error("fuera de desarrollo la cookie deberia exigir Secure")
	}
}
