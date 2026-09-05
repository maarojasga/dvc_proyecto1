package user

import "testing"

func TestNormalizarCorreo(t *testing.T) {
	casos := map[string]string{
		"  Ana@Example.COM ":  "ana@example.com",
		"ana@example.com":     "ana@example.com",
		"\tANA@EXAMPLE.com\n": "ana@example.com",
	}
	for entrada, esperado := range casos {
		if obtenido := NormalizarCorreo(entrada); obtenido != esperado {
			t.Errorf("NormalizarCorreo(%q) = %q; se esperaba %q", entrada, obtenido, esperado)
		}
	}
}

func TestValidarCorreo(t *testing.T) {
	validos := []string{"ana@example.com", "a.b+etiqueta@sub.example.co"}
	for _, c := range validos {
		if err := ValidarCorreo(c); err != nil {
			t.Errorf("ValidarCorreo(%q) = %v; se esperaba nil", c, err)
		}
	}

	invalidos := []string{
		"",
		"sin-arroba",
		"sin@dominio",           // el dominio necesita al menos un punto
		"Ana <ana@example.com>", // solo se acepta la direccion desnuda
		"ana@example.com ",      // debe llegar ya normalizado
	}
	for _, c := range invalidos {
		if err := ValidarCorreo(c); err == nil {
			t.Errorf("ValidarCorreo(%q) = nil; se esperaba error", c)
		}
	}
}

func TestValidarClave(t *testing.T) {
	if err := ValidarClave("clave-larga-y-valida"); err != nil {
		t.Errorf("una clave de longitud suficiente deberia pasar: %v", err)
	}
	cortas := []string{"", "   ", "corta", "123456789"}
	for _, c := range cortas {
		if err := ValidarClave(c); err == nil {
			t.Errorf("ValidarClave(%q) = nil; se esperaba error", c)
		}
	}
	larga := make([]byte, LongitudMaximaClave+1)
	for i := range larga {
		larga[i] = 'a'
	}
	if err := ValidarClave(string(larga)); err == nil {
		t.Error("una clave por encima del maximo deberia rechazarse")
	}
}

func TestValidarNombre(t *testing.T) {
	if err := ValidarNombre("  Ana  "); err != nil {
		t.Errorf("un nombre con espacios alrededor deberia aceptarse: %v", err)
	}
	if err := ValidarNombre("   "); err == nil {
		t.Error("un nombre en blanco deberia rechazarse")
	}
}
