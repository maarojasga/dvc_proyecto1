package badge

import (
	"fmt"
	"strings"
	"time"
)

// El alcance mínimo pide que la insignia tenga imagen, no solo un registro.
// Se dibuja en SVG y no en PNG por tres razones: no arrastra dependencias de
// rasterizado al backend, es el mismo archivo en cualquier tamaño (una
// credencial se mira en la web y se imprime en un CV), y el resultado es
// determinista, así que se puede comparar byte a byte en una prueba.

// ClaveDeImagen es la ruta del objeto donde vive la imagen de una insignia.
//
// Va indexada por el código de verificación y no por el identificador del
// estudiante: la imagen se sirve desde la página pública de verificación, y
// una clave derivada de datos de la persona filtraría en la propia URL lo que
// la insignia se cuida de no exponer.
func ClaveDeImagen(codigoVerificacion string) string {
	return "badges/" + codigoVerificacion + ".svg"
}

// TipoDeImagen es el content-type con el que se almacena y se sirve.
const TipoDeImagen = "image/svg+xml"

// RenderizarSVG dibuja la insignia.
//
// Solo entra lo que la verificación pública ya muestra —curso, fecha y
// código—: el nombre y el correo del estudiante quedan fuera a propósito,
// porque la imagen se comparte y su URL es pública.
func RenderizarSVG(curso string, emitida time.Time, codigoVerificacion string) []byte {
	titulo := recortar(escapar(curso), 42)
	fecha := emitida.Format("2 Jan 2006")
	codigo := escapar(codigoVerificacion)

	var b strings.Builder
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 480 480" width="480" height="480" role="img" aria-label="Insignia del curso %s">`, titulo)
	b.WriteString(`<title>Insignia verificable</title>`)
	b.WriteString(`<defs><linearGradient id="fondo" x1="0" y1="0" x2="0" y2="1">` +
		`<stop offset="0" stop-color="#1e3a8a"/><stop offset="1" stop-color="#0f172a"/></linearGradient></defs>`)

	// Disco exterior y anillo.
	b.WriteString(`<circle cx="240" cy="220" r="200" fill="url(#fondo)"/>`)
	b.WriteString(`<circle cx="240" cy="220" r="182" fill="none" stroke="#fbbf24" stroke-width="6"/>`)

	// Cintas inferiores.
	b.WriteString(`<path d="M170 370 L140 470 L205 440 L240 470 L240 370 Z" fill="#b45309"/>`)
	b.WriteString(`<path d="M310 370 L340 470 L275 440 L240 470 L240 370 Z" fill="#d97706"/>`)

	// Texto. text-anchor=middle centra sin medir tipografías.
	b.WriteString(`<g text-anchor="middle" font-family="Georgia, 'Times New Roman', serif" fill="#f8fafc">`)
	b.WriteString(`<text x="240" y="150" font-size="26" letter-spacing="3" fill="#fbbf24">INSIGNIA</text>`)
	fmt.Fprintf(&b, `<text x="240" y="215" font-size="26" font-weight="bold">%s</text>`, titulo)
	fmt.Fprintf(&b, `<text x="240" y="258" font-size="18" fill="#cbd5f5">%s</text>`, escapar(fecha))
	fmt.Fprintf(&b, `<text x="240" y="320" font-size="13" font-family="monospace" fill="#94a3b8">%s</text>`, codigo)
	b.WriteString(`</g></svg>`)

	return []byte(b.String())
}

// escapar neutraliza el título del curso, que lo escribe un profesor: sin
// esto, un título con "</text><script>" convertiría la imagen —servida desde
// el almacén y abierta en el navegador— en un XSS.
func escapar(s string) string {
	r := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;", "'", "&apos;")
	return r.Replace(s)
}

// recortar limita el largo visible sin partir una entidad XML por la mitad.
func recortar(s string, maximo int) string {
	runas := []rune(s)
	if len(runas) <= maximo {
		return s
	}
	corte := string(runas[:maximo])
	// Si el corte cayó dentro de "&amp;" u otra entidad, se retrocede hasta
	// el "&" para no dejar XML inválido.
	if i := strings.LastIndex(corte, "&"); i >= 0 && !strings.Contains(corte[i:], ";") {
		corte = corte[:i]
	}
	return strings.TrimSpace(corte) + "…"
}
