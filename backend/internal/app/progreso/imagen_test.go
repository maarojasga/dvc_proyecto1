package progreso

import (
	"encoding/xml"
	"strings"
	"testing"
	"time"
)

var emision = time.Date(2026, 3, 15, 12, 0, 0, 0, time.UTC)

// El SVG tiene que ser XML válido, porque va a un navegador.
func TestLaImagenEsSVGValido(t *testing.T) {
	svg := svgDeInsignia("Introducción a la nube", "ABCD-1234-EFGH", emision)
	var nodo struct{ XMLName xml.Name }
	if err := xml.Unmarshal(svg, &nodo); err != nil {
		t.Fatalf("el SVG no es XML válido: %v\n%s", err, svg)
	}
	if nodo.XMLName.Local != "svg" {
		t.Errorf("raíz %q, esperaba svg", nodo.XMLName.Local)
	}
}

// El título lo escribe un profesor, así que puede traer caracteres que rompen
// XML. Si no se escapan, el SVG deja de ser válido —o peor, se convierte en un
// vector de inyección en la página que lo incruste.
func TestElTituloSeEscapa(t *testing.T) {
	svg := svgDeInsignia(`Cloud <script>alert(1)</script> & "más"`, "X-1", emision)
	var nodo struct{ XMLName xml.Name }
	if err := xml.Unmarshal(svg, &nodo); err != nil {
		t.Fatalf("un título con caracteres especiales rompió el SVG: %v\n%s", err, svg)
	}
	if strings.Contains(string(svg), "<script>") {
		t.Error("el SVG contiene una etiqueta script sin escapar")
	}
}

// La imagen se comparte, así que no puede llevar datos personales. La
// condición de emisión lo pide explícitamente.
func TestLaImagenNoLlevaDatosPersonales(t *testing.T) {
	svg := string(svgDeInsignia("Curso de prueba", "COD-123", emision))
	for _, prohibido := range []string{"@", "estudiante@", "Juan", "correo"} {
		if strings.Contains(svg, prohibido) {
			t.Errorf("el SVG contiene %q", prohibido)
		}
	}
}

func TestLaImagenLlevaCursoFechaYCodigo(t *testing.T) {
	svg := string(svgDeInsignia("Fundamentos de Go", "VERIF-999", emision))
	for _, esperado := range []string{"Fundamentos de Go", "2026-03-15", "VERIF-999"} {
		if !strings.Contains(svg, esperado) {
			t.Errorf("el SVG no contiene %q", esperado)
		}
	}
}

// La misma insignia se ve igual siempre: el color sale del código, no del azar.
func TestElColorEsEstable(t *testing.T) {
	a, b := colorDe("MISMO-CODIGO")
	c, d := colorDe("MISMO-CODIGO")
	if a != c || b != d {
		t.Fatalf("el color cambió entre llamadas: (%s,%s) vs (%s,%s)", a, b, c, d)
	}
	// Y códigos distintos reparten entre la paleta, no siempre al mismo.
	vistos := map[string]bool{}
	for _, cod := range []string{"a", "b", "c", "d", "e", "f", "g", "h", "i", "j", "k", "l"} {
		f, _ := colorDe(cod)
		vistos[f] = true
	}
	if len(vistos) < 3 {
		t.Errorf("solo se usaron %d colores de %d; el reparto está sesgado", len(vistos), len(paleta))
	}
}

func TestUnTituloLargoNoDesbordaLaInsignia(t *testing.T) {
	largo := "Un título de curso extremadamente largo que no cabría de ninguna manera en el círculo de la insignia"
	svg := string(svgDeInsignia(largo, "COD", emision))
	var nodo struct{ XMLName xml.Name }
	if err := xml.Unmarshal([]byte(svg), &nodo); err != nil {
		t.Fatalf("SVG inválido: %v", err)
	}
	// Se recorta: si apareciera completo, se saldría del dibujo.
	if strings.Contains(svg, largo) {
		t.Error("el título largo no se recortó")
	}
	if !strings.Contains(svg, "…") {
		t.Error("no hay marca de recorte, así que el recorte no se ve")
	}
}

func TestPartirEnLineas(t *testing.T) {
	casos := []struct {
		texto  string
		ancho  int
		lineas int
	}{
		{"corto", 20, 1},
		{"dos palabras aquí", 12, 2},
		{"", 10, 1},
		// Una palabra que no cabe se deja entera en su línea antes que
		// cortarla por la mitad.
		{"palabraquenocabeenningunalinea", 5, 1},
	}
	for _, c := range casos {
		got := partirEnLineas(c.texto, c.ancho)
		if len(got) != c.lineas {
			t.Errorf("partirEnLineas(%q, %d) dio %d líneas (%q), esperaba %d", c.texto, c.ancho, len(got), got, c.lineas)
		}
	}
}

// Más de tres líneas no cabe en el círculo, así que el resto se colapsa en la
// tercera. Sin este tope un título largo se saldría del dibujo.
func TestNuncaMasDeTresLineas(t *testing.T) {
	largo := "uno dos tres cuatro cinco seis siete ocho nueve diez once doce trece"
	got := partirEnLineas(largo, 12)
	if len(got) > 3 {
		t.Fatalf("dio %d líneas: %q", len(got), got)
	}
	if !strings.HasSuffix(got[len(got)-1], "…") {
		t.Errorf("la última línea no marca que se recortó: %q", got[len(got)-1])
	}
}

func TestLaClaveDeImagenSaleDelCodigo(t *testing.T) {
	if got := claveDeImagen("ABC-123"); got != "badges/ABC-123.svg" {
		t.Errorf("clave %q", got)
	}
}
