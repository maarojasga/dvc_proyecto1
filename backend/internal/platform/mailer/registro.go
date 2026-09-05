package mailer

import (
	"context"
	"log/slog"
)

// Registro es un notificador que solo escribe en el log en lugar de enviar.
//
// Sirve para las pruebas y para arrancar la API sin SMTP: el enlace queda
// visible en el log del contenedor, nunca en la respuesta HTTP, que seguiria
// permitiendo activar cuentas ajenas.
type Registro struct{ log *slog.Logger }

// NuevoRegistro construye el notificador de bitacora.
func NuevoRegistro(log *slog.Logger) *Registro { return &Registro{log: log} }

// EnviarVerificacionCorreo registra el enlace de verificacion.
func (r *Registro) EnviarVerificacionCorreo(ctx context.Context, destino, nombre, token string) error {
	r.log.WarnContext(ctx, "SMTP no configurado: verificacion no enviada",
		"destino", destino, "token", token)
	return nil
}

// EnviarRecuperacionClave registra el enlace de recuperacion.
func (r *Registro) EnviarRecuperacionClave(ctx context.Context, destino, nombre, token string) error {
	r.log.WarnContext(ctx, "SMTP no configurado: recuperacion no enviada",
		"destino", destino, "token", token)
	return nil
}
