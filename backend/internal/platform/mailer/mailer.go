// Package mailer envía correo transaccional (verificación de cuenta,
// recuperación de contraseña) vía SMTP. En desarrollo apunta a Mailpit.
package mailer

import (
	"fmt"
	"net/smtp"
)

// Sender es el transporte que entrega el mensaje ya compuesto. Separarlo de
// Mailer permite sustituir SMTP por un buzón en memoria en las pruebas, que
// necesitan leer el enlace del correo porque la API nunca lo devuelve.
type Sender interface {
	Send(to, subject, body string) error
}

type Mailer struct {
	sender Sender
	from   string
}

// New construye el mailer sobre SMTP. En desarrollo apunta a Mailpit.
func New(host, port, from string) *Mailer {
	return &Mailer{sender: smtpSender{host: host, port: port, from: from}, from: from}
}

// NewWithSender construye el mailer sobre un transporte propio.
func NewWithSender(s Sender, from string) *Mailer {
	return &Mailer{sender: s, from: from}
}

func (m *Mailer) Send(to, subject, body string) error {
	return m.sender.Send(to, subject, body)
}

type smtpSender struct {
	host string
	port string
	from string
}

func (s smtpSender) Send(to, subject, body string) error {
	addr := fmt.Sprintf("%s:%s", s.host, s.port)
	msg := fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\n%s",
		s.from, to, subject, body)
	// Mailpit acepta SMTP sin autenticación en desarrollo.
	return smtp.SendMail(addr, nil, s.from, []string{to}, []byte(msg))
}
