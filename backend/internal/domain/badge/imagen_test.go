package badge_test

import (
	"encoding/xml"
	"strings"
	"testing"
	"time"

	"github.com/DES-SOLUCIONES-CLOUD/proyecto-1/backend/internal/domain/badge"
)

func TestLaImagenEsSVGValidoYLlevaCursoFechaYCodigo(t *testing.T) {
	emitida := time.Date(2026, 3, 15, 12, 0, 0, 0, time.UTC)
	svg := badge.RenderizarSVG("Fundamentos de la nube", emitida, "abc123xyz")

	if err := xml.Unmarshal(svg, new(struct{})); err != nil {
		t.Fatalf("el SVG generado no es XML válido: %v", err)
	}
	for _, esperado := range []string{"Fundamentos de la nube", "15 Mar 2026", "abc123xyz"} {
		if !strings.Contains(string(svg), esperado) {
			t.Errorf("la imagen no contiene %q", esperado)
		}
	}
}

// El título lo escribe un profesor y la imagen la abre el navegador desde el
// almacén: sin escapado, un título con etiquetas sería XSS servido por la
// plataforma.
func TestUnTituloConMarcadoNoSeCuelaEnLaImagen(t *testing.T) {
	svg := string(badge.RenderizarSVG(`</text><script>alert("x")</script>`, time.Now().UTC(), "codigo"))

	if strings.Contains(svg, "<script>") {
		t.Fatalf("el marcado del título llegó sin escapar a la imagen:\n%s", svg)
	}
	if err := xml.Unmarshal([]byte(svg), new(struct{})); err != nil {
		t.Fatalf("el SVG con un título hostil dejó de ser XML válido: %v", err)
	}
}

// La imagen se comparte y su URL es pública: no puede llevar datos de la
// persona, igual que la respuesta de verificación.
func TestLaImagenNoLlevaDatosDelEstudiante(t *testing.T) {
	svg := string(badge.RenderizarSVG("Curso de prueba", time.Now().UTC(), "codigo-publico"))

	for _, prohibido := range []string{"@", "estudiante", "student_id"} {
		if strings.Contains(strings.ToLower(svg), prohibido) {
			t.Errorf("la imagen contiene %q, que no debería aparecer", prohibido)
		}
	}
}

// La clave del objeto deriva del código público, no del estudiante ni de la
// inscripción: la URL de la imagen es tan pública como la de verificación.
func TestLaClaveDeLaImagenDerivaDelCodigoPublico(t *testing.T) {
	if clave := badge.ClaveDeImagen("codigo-publico"); clave != "badges/codigo-publico.svg" {
		t.Errorf("clave inesperada: %q", clave)
	}
}

// Un título muy largo se recorta sin romper el XML.
func TestUnTituloLargoSeRecortaSinRomperElSVG(t *testing.T) {
	largo := strings.Repeat("Arquitecturas distribuidas y ", 10)
	svg := badge.RenderizarSVG(largo, time.Now().UTC(), "codigo")

	if err := xml.Unmarshal(svg, new(struct{})); err != nil {
		t.Fatalf("el recorte dejó XML inválido: %v", err)
	}
	if !strings.Contains(string(svg), "…") {
		t.Error("se esperaba la marca de recorte en un título largo")
	}
}
