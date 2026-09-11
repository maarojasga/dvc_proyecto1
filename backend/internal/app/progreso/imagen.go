package progreso

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"html"
	"strings"
	"time"
)

// La insignia lleva imagen porque el alcance minimo la pide, y se genera en el
// servidor al emitirla. Se guarda en el almacen de objetos, no en Postgres:
// la regla de arquitectura es que ningun binario vive en la base relacional, y
// las imagenes de insignias estan nombradas ahi explicitamente.
//
// Es un SVG y no un PNG a proposito: no hace falta una biblioteca de imagen,
// pesa unos cientos de bytes, escala sin perder nitidez en cualquier tamano y
// se puede incrustar en un certificado o en un perfil.

// AlmacenDeInsignias es lo unico que la emision necesita del almacenamiento.
//
// Va estrecho por la misma razon que en la entrega de contenido: el servicio
// de progreso no tiene por que poder firmar subidas de material de curso.
type AlmacenDeInsignias interface {
	SubirBytes(ctx context.Context, objectKey string, contenido []byte, contentType string) error
	PresignedGetURL(ctx context.Context, objectKey string, expiry time.Duration, filename string) (string, error)
}

// vigenciaImagen es lo que dura la URL de la imagen cuando se firma. Es mas
// larga que la de un recurso de curso porque una insignia se comparte: alguien
// puede pegarla en un correo y abrirla un rato despues.
const vigenciaImagen = 24 * time.Hour

// claveDeImagen es donde vive la imagen de una insignia. Se indexa por el
// codigo de verificacion, que es publico y estable.
func claveDeImagen(codigo string) string {
	return "badges/" + codigo + ".svg"
}

// paleta son los pares (fondo, borde) entre los que se reparte la insignia.
// Que dos cursos distintos se vean distintos no es decorativo: ayuda a
// reconocer de un vistazo que una insignia no es la que se esperaba.
var paleta = [][2]string{
	{"#1e3a5f", "#4a90d9"},
	{"#3d2b56", "#a06cd5"},
	{"#1f4d3c", "#4caf82"},
	{"#5c3317", "#d98b45"},
	{"#4a1c2e", "#d95f7e"},
	{"#2b3d4f", "#7fa8c4"},
}

// colorDe reparte el color a partir del curso, de forma estable: la misma
// insignia se ve igual cada vez que se pide.
func colorDe(semilla string) (fondo, borde string) {
	suma := sha256.Sum256([]byte(semilla))
	i := binary.BigEndian.Uint32(suma[:4]) % uint32(len(paleta))
	return paleta[i][0], paleta[i][1]
}

// svgDeInsignia dibuja la insignia.
//
// Lleva el titulo del curso, la fecha y el codigo de verificacion, y NO lleva
// el nombre ni el correo del estudiante: la imagen se comparte, y la condicion
// de emision pide que lo publico no exponga datos personales. Quien quiera
// saber a quien pertenece usa la URL de verificacion.
func svgDeInsignia(tituloCurso, codigo string, emitida time.Time) []byte {
	fondo, borde := colorDe(codigo)
	titulo := recortar(tituloCurso, 42)
	fecha := emitida.Format("2006-01-02")

	// El texto va escapado porque el titulo lo escribe un profesor: sin esto,
	// un titulo con < o & produciria un SVG roto o algo peor.
	var b strings.Builder
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 400 400" width="400" height="400" role="img" aria-label="Insignia de %s">`, html.EscapeString(titulo))
	fmt.Fprintf(&b, `<title>Insignia de %s</title>`, html.EscapeString(titulo))
	fmt.Fprintf(&b, `<circle cx="200" cy="200" r="190" fill="%s"/>`, fondo)
	fmt.Fprintf(&b, `<circle cx="200" cy="200" r="190" fill="none" stroke="%s" stroke-width="10"/>`, borde)
	fmt.Fprintf(&b, `<circle cx="200" cy="200" r="165" fill="none" stroke="%s" stroke-width="2" stroke-opacity="0.5"/>`, borde)
	// Una cinta simple, para que se lea como insignia y no como un circulo.
	// Va dentro del circulo: en y=372 el circulo mide 81 de medio ancho, asi
	// que 150..250 cabe sin asomar por el borde.
	fmt.Fprintf(&b, `<path d="M150 300 L150 372 L200 348 L250 372 L250 300 Z" fill="%s"/>`, borde)
	fmt.Fprintf(&b, `<text x="200" y="130" text-anchor="middle" font-family="system-ui,sans-serif" font-size="22" fill="%s" letter-spacing="3">CURSO APROBADO</text>`, borde)

	// El titulo se parte en lineas para que quepa en el circulo.
	lineas := partirEnLineas(titulo, 20)
	y := 200 - (len(lineas)-1)*16
	for _, linea := range lineas {
		fmt.Fprintf(&b, `<text x="200" y="%d" text-anchor="middle" font-family="system-ui,sans-serif" font-size="28" font-weight="600" fill="#ffffff">%s</text>`, y, html.EscapeString(linea))
		y += 34
	}

	fmt.Fprintf(&b, `<text x="200" y="258" text-anchor="middle" font-family="system-ui,sans-serif" font-size="16" fill="#ffffff" fill-opacity="0.85">%s</text>`, fecha)
	fmt.Fprintf(&b, `<text x="200" y="284" text-anchor="middle" font-family="ui-monospace,monospace" font-size="12" fill="%s">%s</text>`, borde, html.EscapeString(codigo))
	b.WriteString(`</svg>`)
	return []byte(b.String())
}

func recortar(s string, max int) string {
	r := []rune(strings.TrimSpace(s))
	if len(r) <= max {
		return string(r)
	}
	return strings.TrimSpace(string(r[:max-1])) + "…"
}

// partirEnLineas reparte el texto en lineas de como mucho ancho runas, sin
// cortar palabras salvo que una sola no quepa.
func partirEnLineas(s string, ancho int) []string {
	palabras := strings.Fields(s)
	if len(palabras) == 0 {
		return []string{""}
	}
	var lineas []string
	actual := ""
	for _, p := range palabras {
		candidata := p
		if actual != "" {
			candidata = actual + " " + p
		}
		if len([]rune(candidata)) <= ancho {
			actual = candidata
			continue
		}
		if actual != "" {
			lineas = append(lineas, actual)
		}
		actual = p
	}
	if actual != "" {
		lineas = append(lineas, actual)
	}
	// Mas de tres lineas no cabe en el circulo.
	if len(lineas) > 3 {
		lineas = append(lineas[:2], recortar(strings.Join(lineas[2:], " "), ancho))
	}
	return lineas
}
