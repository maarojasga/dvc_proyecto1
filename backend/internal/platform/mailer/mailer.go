// Package mailer envía correo transaccional (verificación de cuenta,
// recuperación de contraseña) vía SMTP. En desarrollo apunta a Mailpit.
package mailer

import (
	"fmt"
	"net/smtp"
)

type Mailer struct {
	host string
	port string
	from string
}

func New(host, port, from string) *Mailer {
	return &Mailer{host: host, port: port, from: from}
}

func (m *Mailer) Send(to, subject, body string) error {
	addr := fmt.Sprintf("%s:%s", m.host, m.port)
	msg := fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\n%s",
		m.from, to, subject, body)
	// Mailpit acepta SMTP sin autenticación en desarrollo.
	return smtp.SendMail(addr, nil, m.from, []string{to}, []byte(msg))
}
