package user

import (
	"net/mail"
	"strings"
	"unicode/utf8"
)

// Politica de claves del MVP. No se exige composicion por clases de caracteres
// (contraproducente segun NIST SP 800-63B); se exige longitud y se rechazan
// las claves compuestas solo por espacios.
const (
	LongitudMinimaClave  = 10
	LongitudMaximaClave  = 128
	LongitudMaximaNombre = 120
	LongitudMaximaCorreo = 254
)

// NormalizarCorreo deja el correo en la forma canonica que se persiste:
// sin espacios alrededor y en minusculas. Es la unica forma que llega a la
// base, de modo que el UNIQUE de la tabla basta para impedir duplicados que
// solo difieren en mayusculas.
func NormalizarCorreo(correo string) string {
	return strings.ToLower(strings.TrimSpace(correo))
}

// ValidarCorreo comprueba el formato y la longitud del correo ya normalizado.
func ValidarCorreo(correo string) error {
	if correo == "" || len(correo) > LongitudMaximaCorreo {
		return ErrCorreoInvalido
	}
	direccion, err := mail.ParseAddress(correo)
	if err != nil || direccion.Address != correo {
		return ErrCorreoInvalido
	}
	// mail.ParseAddress acepta direcciones sin dominio con punto; para el
	// registro publico se exige un dominio con al menos un punto.
	arroba := strings.LastIndex(correo, "@")
	if arroba < 1 || !strings.Contains(correo[arroba+1:], ".") {
		return ErrCorreoInvalido
	}
	return nil
}

// ValidarNombre comprueba que el nombre visible no venga vacio.
func ValidarNombre(nombre string) error {
	nombre = strings.TrimSpace(nombre)
	if nombre == "" || utf8.RuneCountInString(nombre) > LongitudMaximaNombre {
		return ErrNombreInvalido
	}
	return nil
}

// ValidarClave aplica la politica minima de claves.
func ValidarClave(clave string) error {
	if strings.TrimSpace(clave) == "" {
		return ErrClaveInsegura
	}
	if n := utf8.RuneCountInString(clave); n < LongitudMinimaClave || n > LongitudMaximaClave {
		return ErrClaveInsegura
	}
	return nil
}
