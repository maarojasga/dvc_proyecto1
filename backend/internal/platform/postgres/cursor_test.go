package postgres

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestElCursorSobreviveLaIdaYVuelta(t *testing.T) {
	original := Cursor{
		Instante: time.Date(2026, 3, 15, 12, 34, 56, 789000000, time.UTC),
		ID:       uuid.MustParse("6ba7b810-9dad-11d1-80b4-00c04fd430c8"),
	}
	vuelta, err := DecodificarCursor(original.Codificar())
	if err != nil {
		t.Fatalf("decodificar: %v", err)
	}
	if !vuelta.Instante.Equal(original.Instante) {
		t.Errorf("instante %v, esperaba %v", vuelta.Instante, original.Instante)
	}
	if vuelta.ID != original.ID {
		t.Errorf("id %v, esperaba %v", vuelta.ID, original.ID)
	}
}

// El cursor va en una URL, así que no puede necesitar escapes.
func TestElCursorCabeEnUnaURLSinEscapar(t *testing.T) {
	c := Cursor{Instante: time.Now().UTC(), ID: uuid.New()}
	s := c.Codificar()
	for _, r := range s {
		esValido := (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_'
		if !esValido {
			t.Fatalf("el cursor %q tiene el carácter %q, que hay que escapar en una URL", s, r)
		}
	}
}

// Un cursor corrupto tiene que fallar y no interpretarse como el principio:
// devolver la primera página en silencio es peor que un error, porque el
// cliente creería estar avanzando.
func TestUnCursorCorruptoFalla(t *testing.T) {
	for _, entrada := range []string{
		"",
		"no-es-base64-!!!",
		"c2luLXNlcGFyYWRvcg",      // base64 válido, sin el separador
		"MTIzfG5vLWVzLXVuLXV1aWQ", // instante bueno, id malo
		"bm8tZXMtdW4tbnVtZXJvfDZiYTdiODEwLTlkYWQtMTFkMS04MGI0LTAwYzA0ZmQ0MzBjOA", // id bueno, instante malo
	} {
		if _, err := DecodificarCursor(entrada); err == nil {
			t.Errorf("DecodificarCursor(%q) no falló", entrada)
		}
	}
}
