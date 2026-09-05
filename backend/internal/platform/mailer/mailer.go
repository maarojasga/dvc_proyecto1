// Package mailer entrega los correos transaccionales de identidad.
//
// En local el destino es Mailpit, levantado por docker-compose; en produccion
// cualquier SMTP con credenciales externas.
package mailer

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"mime"
	"net"
	"net/smtp"
	"net/url"
	"strings"
	"time"
)

// Config describe el servidor SMTP y como construir los enlaces del correo.
type Config struct {
	Direccion   string // host:puerto
	Remitente   string
	Usuario     string
	Clave       string
	URLFrontend string // base publica del frontend, para los enlaces
}

// SMTP implementa user.Notificador sobre un servidor SMTP.
type SMTP struct {
	cfg Config
	log *slog.Logger
}

// Nuevo construye el notificador.
func Nuevo(cfg Config, log *slog.Logger) *SMTP {
	return &SMTP{cfg: cfg, log: log}
}

// EnviarVerificacionCorreo manda el enlace de verificacion de la cuenta.
func (s *SMTP) EnviarVerificacionCorreo(ctx context.Context, destino, nombre, token string) error {
	enlace := s.enlace("/verificar-correo", token)
	cuerpo := fmt.Sprintf(`Hola %s:

Para activar tu cuenta en la Plataforma MOOC, abre este enlace:

%s

El enlace vence en 24 horas. Si no creaste esta cuenta, ignora el mensaje.
`, nombre, enlace)
	return s.enviar(ctx, destino, "Verifica tu correo", cuerpo)
}

// EnviarRecuperacionClave manda el enlace de recuperacion de clave.
func (s *SMTP) EnviarRecuperacionClave(ctx context.Context, destino, nombre, token string) error {
	enlace := s.enlace("/recuperar-clave", token)
	cuerpo := fmt.Sprintf(`Hola %s:

Recibimos una solicitud para restablecer tu clave. Abre este enlace:

%s

El enlace vence en una hora y solo puede usarse una vez. Si no lo pediste,
no hagas nada: tu clave actual sigue siendo valida.
`, nombre, enlace)
	return s.enviar(ctx, destino, "Restablece tu clave", cuerpo)
}

// enlace arma la URL del frontend con el token como parametro.
func (s *SMTP) enlace(ruta, token string) string {
	base := strings.TrimSuffix(s.cfg.URLFrontend, "/")
	return fmt.Sprintf("%s%s?token=%s", base, ruta, url.QueryEscape(token))
}

func (s *SMTP) enviar(ctx context.Context, destino, asunto, cuerpo string) error {
	var mensaje bytes.Buffer
	fmt.Fprintf(&mensaje, "From: %s\r\n", s.cfg.Remitente)
	fmt.Fprintf(&mensaje, "To: %s\r\n", destino)
	fmt.Fprintf(&mensaje, "Subject: %s\r\n", mime.QEncoding.Encode("utf-8", asunto))
	fmt.Fprintf(&mensaje, "Date: %s\r\n", time.Now().Format(time.RFC1123Z))
	mensaje.WriteString("MIME-Version: 1.0\r\n")
	mensaje.WriteString("Content-Type: text/plain; charset=utf-8\r\n\r\n")
	mensaje.WriteString(strings.ReplaceAll(cuerpo, "\n", "\r\n"))

	var auth smtp.Auth
	if s.cfg.Usuario != "" {
		host, _, err := net.SplitHostPort(s.cfg.Direccion)
		if err != nil {
			return fmt.Errorf("mailer: direccion invalida: %w", err)
		}
		auth = smtp.PlainAuth("", s.cfg.Usuario, s.cfg.Clave, host)
	}

	if err := smtp.SendMail(s.cfg.Direccion, auth, s.cfg.Remitente, []string{destino}, mensaje.Bytes()); err != nil {
		return fmt.Errorf("mailer: no se pudo enviar a %s: %w", destino, err)
	}
	s.log.InfoContext(ctx, "correo enviado", "destino", destino, "asunto", asunto)
	return nil
}
