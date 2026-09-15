package media

import (
	"strings"
	"testing"
)

func TestSeleccionarCalidadesNuncaSubeDeResolucion(t *testing.T) {
	casos := []struct {
		alturaOriginal int
		esperadas      []int
	}{
		{1080, []int{1080, 720, 480, 360}},
		{720, []int{720, 480, 360}},
		{480, []int{480, 360}},
		{360, []int{360}},
		// Un original por debajo de toda la escalera se emite tal cual, en una
		// sola variante: reescalar hacia arriba no añade detalle.
		{240, []int{240}},
	}
	for _, c := range casos {
		elegidas := seleccionarCalidades(c.alturaOriginal)
		if len(elegidas) != len(c.esperadas) {
			t.Errorf("original %dp: se eligieron %d calidades, se esperaban %d", c.alturaOriginal, len(elegidas), len(c.esperadas))
			continue
		}
		for i, r := range elegidas {
			if r.Height != c.esperadas[i] {
				t.Errorf("original %dp: calidad %d = %dp, se esperaba %dp", c.alturaOriginal, i, r.Height, c.esperadas[i])
			}
			if r.Height > c.alturaOriginal {
				t.Errorf("original %dp: no debe emitirse una variante de %dp", c.alturaOriginal, r.Height)
			}
		}
	}
}

func TestAnchoEscaladoConservaAspectoYEsPar(t *testing.T) {
	casos := []struct {
		anchoO, altoO, alto, esperado int
	}{
		{1920, 1080, 720, 1280},
		{1920, 1080, 480, 854}, // 853.3 redondea a 853 e sube al par siguiente
		{1280, 720, 360, 640},
		{1080, 1920, 720, 406}, // vertical
	}
	for _, c := range casos {
		got := anchoEscalado(c.anchoO, c.altoO, c.alto)
		if got != c.esperado {
			t.Errorf("anchoEscalado(%d,%d,%d) = %d; se esperaba %d", c.anchoO, c.altoO, c.alto, got, c.esperado)
		}
		if got%2 != 0 {
			t.Errorf("el ancho debe ser par para 4:2:0, llegó %d", got)
		}
	}
}

func TestMasterDeVideoDeclaraResolucionCompleta(t *testing.T) {
	master := masterDeVideo(seleccionarCalidades(1080), 1920, 1080)

	if !strings.HasPrefix(master, "#EXTM3U\n") {
		t.Errorf("la lista maestra debe empezar por #EXTM3U:\n%s", master)
	}
	// RESOLUTION necesita ANCHOxALTO. Emitirlo sin el ancho ("x1080") no es un
	// valor válido y deja al reproductor sin saber a qué variante cambiar.
	if strings.Contains(master, "RESOLUTION=x") {
		t.Errorf("RESOLUTION sin ancho es inválido:\n%s", master)
	}
	for _, esperado := range []string{
		"RESOLUTION=1920x1080", "RESOLUTION=1280x720", "RESOLUTION=854x480", "RESOLUTION=640x360",
	} {
		if !strings.Contains(master, esperado) {
			t.Errorf("falta %q en la lista maestra:\n%s", esperado, master)
		}
	}
	for _, esperado := range []string{"h1080.m3u8", "h720.m3u8", "h480.m3u8", "h360.m3u8"} {
		if !strings.Contains(master, esperado) {
			t.Errorf("falta la variante %q:\n%s", esperado, master)
		}
	}
	// Cada #EXT-X-STREAM-INF debe ir seguido de su URI en la línea siguiente.
	lineas := strings.Split(strings.TrimSpace(master), "\n")
	for i, l := range lineas {
		if !strings.HasPrefix(l, "#EXT-X-STREAM-INF") {
			continue
		}
		if i+1 >= len(lineas) || strings.HasPrefix(lineas[i+1], "#") {
			t.Errorf("la línea %d declara una variante sin URI:\n%s", i, master)
		}
	}
}

func TestMasterDeAudio(t *testing.T) {
	master := masterDeAudio()
	if !strings.Contains(master, "audio.m3u8") || !strings.Contains(master, "#EXT-X-STREAM-INF") {
		t.Errorf("la lista maestra de audio no declara su variante:\n%s", master)
	}
	// Sin video no debe anunciarse resolución alguna.
	if strings.Contains(master, "RESOLUTION") {
		t.Errorf("una pista de audio no debería declarar RESOLUTION:\n%s", master)
	}
}

func TestParsearDimensiones(t *testing.T) {
	ancho, alto, err := parsearDimensiones("1920x1080\n")
	if err != nil || ancho != 1920 || alto != 1080 {
		t.Errorf("parsearDimensiones = (%d,%d,%v)", ancho, alto, err)
	}
	for _, malo := range []string{"", "1920", "axb", "1920x"} {
		if _, _, err := parsearDimensiones(malo); err == nil {
			t.Errorf("parsearDimensiones(%q) debería fallar", malo)
		}
	}
}
