package postgres

import (
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Paginación por cursor.
//
// OFFSET no sirve para un catálogo que cambia: si se publica un curso mientras
// alguien pasa de página, las filas se desplazan y el lector se salta una o ve
// otra dos veces. Y el coste crece con el desplazamiento, porque la base tiene
// que contar y descartar todo lo anterior.
//
// El cursor apunta a la última fila entregada, y la consulta pide "lo
// siguiente a esta". Es estable frente a inserciones y su coste no depende de
// lo lejos que se haya llegado.

// ErrCursorInvalido indica un cursor que no se puede interpretar. Se trata
// como petición inválida y no como "no hay más": un cursor corrupto que se
// interpretara como el principio devolvería la primera página en silencio.
var ErrCursorInvalido = errors.New("postgres: cursor inválido")

// Cursor es la posición de la última fila entregada.
//
// Lleva la clave de orden y el identificador. El identificador es el
// desempate: dos versiones pueden publicarse en el mismo instante, y sin él el
// orden no sería total y la paginación podría repetir o saltarse filas.
type Cursor struct {
	Instante time.Time
	ID       uuid.UUID
}

// Codificar deja el cursor en una cadena opaca para el cliente.
//
// Va en base64 sin relleno para que quepa en una URL sin escapes. Es opaco a
// propósito: que el cliente no lo interprete deja libertad para cambiar el
// orden sin romper a nadie.
func (c Cursor) Codificar() string {
	crudo := fmt.Sprintf("%d|%s", c.Instante.UTC().UnixNano(), c.ID)
	return base64.RawURLEncoding.EncodeToString([]byte(crudo))
}

// DecodificarCursor interpreta lo que devolvió Codificar.
func DecodificarCursor(s string) (Cursor, error) {
	if s == "" {
		return Cursor{}, ErrCursorInvalido
	}
	crudo, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return Cursor{}, ErrCursorInvalido
	}
	partes := strings.SplitN(string(crudo), "|", 2)
	if len(partes) != 2 {
		return Cursor{}, ErrCursorInvalido
	}
	var nanos int64
	if _, err := fmt.Sscanf(partes[0], "%d", &nanos); err != nil {
		return Cursor{}, ErrCursorInvalido
	}
	id, err := uuid.Parse(partes[1])
	if err != nil {
		return Cursor{}, ErrCursorInvalido
	}
	return Cursor{Instante: time.Unix(0, nanos).UTC(), ID: id}, nil
}
