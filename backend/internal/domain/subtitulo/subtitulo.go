// Package subtitulo interpreta pistas WebVTT y deriva de ellas la
// transcripción del material (alcance opcional 5.2).
//
// La transcripción no se pide aparte: se saca de los subtítulos, que es donde
// ya está el texto. Pedir las dos cosas por separado garantizaría que acaben
// diciendo cosas distintas, porque nadie actualiza dos veces lo mismo.
//
// El análisis es deliberadamente tolerante. Un archivo de subtítulos llega de
// una herramienta externa o de un servicio de transcripción automática, y
// rechazar la pista entera por una línea rara dejaría el vídeo sin subtítulos
// por un detalle que el reproductor habría ignorado.
package subtitulo

import (
	"errors"
	"regexp"
	"strconv"
	"strings"
)

// ErrNoEsWebVTT indica un archivo que no empieza por la cabecera del formato.
var ErrNoEsWebVTT = errors.New("subtitulo: el archivo no es WebVTT")

// Entrada es un fragmento de la pista.
type Entrada struct {
	// DesdeMS y HastaMS son milisegundos desde el inicio del medio.
	DesdeMS int
	HastaMS int
	Texto   string
}

// Pista es una lista de entradas en orden.
type Pista []Entrada

// tiempoVTT reconoce "00:01:02.345" y también "01:02.345", que el formato
// admite cuando no hay horas.
var tiempoVTT = regexp.MustCompile(`^(?:(\d+):)?(\d{1,2}):(\d{2})[.,](\d{1,3})$`)

// separadorDeTiempos es la flecha que divide inicio y fin. Puede llevar
// ajustes de posición detrás ("align:start line:90%"), que aquí no interesan.
const separadorDeTiempos = "-->"

// Analizar interpreta un archivo WebVTT.
//
// Exige la cabecera porque es lo único que distingue WebVTT de SRT, que es
// casi idéntico pero usa coma en los milisegundos y numera los fragmentos: un
// SRT entregado como WebVTT lo rechaza el navegador, así que aceptarlo aquí
// solo trasladaría el fallo a la pantalla del estudiante.
func Analizar(contenido string) (Pista, error) {
	texto := strings.ReplaceAll(contenido, "\r\n", "\n")
	if !strings.HasPrefix(strings.TrimLeft(texto, "\ufeff \t"), "WEBVTT") {
		return nil, ErrNoEsWebVTT
	}

	var pista Pista
	var actual *Entrada
	var cuerpo []string

	cerrar := func() {
		if actual == nil {
			return
		}
		actual.Texto = strings.TrimSpace(strings.Join(cuerpo, "\n"))
		if actual.Texto != "" {
			pista = append(pista, *actual)
		}
		actual, cuerpo = nil, nil
	}

	for _, linea := range strings.Split(texto, "\n") {
		recortada := strings.TrimSpace(linea)

		if strings.Contains(recortada, separadorDeTiempos) {
			cerrar()
			desde, hasta, ok := tiempos(recortada)
			if !ok {
				continue
			}
			actual = &Entrada{DesdeMS: desde, HastaMS: hasta}
			continue
		}

		if recortada == "" {
			cerrar()
			continue
		}
		if actual != nil {
			cuerpo = append(cuerpo, recortada)
		}
		// Fuera de un fragmento se ignora todo: la cabecera, los bloques NOTE,
		// los STYLE y el identificador que algunas herramientas ponen antes de
		// los tiempos.
	}
	cerrar()
	return pista, nil
}

func tiempos(linea string) (desde, hasta int, ok bool) {
	partes := strings.SplitN(linea, separadorDeTiempos, 2)
	if len(partes) != 2 {
		return 0, 0, false
	}
	inicio, ok1 := aMilisegundos(strings.TrimSpace(partes[0]))
	// Lo que sigue al tiempo de fin son ajustes de posición del fragmento.
	finYAjustes := strings.Fields(strings.TrimSpace(partes[1]))
	if len(finYAjustes) == 0 {
		return 0, 0, false
	}
	fin, ok2 := aMilisegundos(finYAjustes[0])
	if !ok1 || !ok2 {
		return 0, 0, false
	}
	return inicio, fin, true
}

func aMilisegundos(s string) (int, bool) {
	m := tiempoVTT.FindStringSubmatch(s)
	if m == nil {
		return 0, false
	}
	horas, _ := strconv.Atoi(m[1]) // vacío da 0, que es lo correcto
	minutos, _ := strconv.Atoi(m[2])
	segundos, _ := strconv.Atoi(m[3])

	// Los milisegundos pueden venir con menos de tres cifras: ".5" es medio
	// segundo, no cinco milésimas.
	fraccion := m[4]
	for len(fraccion) < 3 {
		fraccion += "0"
	}
	milis, _ := strconv.Atoi(fraccion)

	return ((horas*60+minutos)*60+segundos)*1000 + milis, true
}

// etiquetas quita el marcado que WebVTT admite dentro del texto de un
// fragmento: <v Locutor>, <i>, <c.clase> y las marcas de tiempo internas.
var etiquetas = regexp.MustCompile(`<[^>]*>`)

// Transcripcion devuelve el texto corrido de la pista.
//
// Es lo que se enseña junto al vídeo y lo que hace el material buscable y
// legible para quien no puede o no quiere reproducirlo. Los fragmentos se unen
// con espacios y no con saltos porque en una pista bien hecha una frase suele
// repartirse entre dos fragmentos: separarlos por líneas partiría las frases.
func (p Pista) Transcripcion() string {
	partes := make([]string, 0, len(p))
	for _, e := range p {
		limpio := strings.TrimSpace(etiquetas.ReplaceAllString(e.Texto, ""))
		// Dentro de un fragmento sí hay saltos de línea reales, que son
		// divisiones de subtítulo y no de discurso.
		limpio = strings.Join(strings.Fields(limpio), " ")
		if limpio != "" {
			partes = append(partes, limpio)
		}
	}
	return strings.Join(partes, " ")
}

// Duracion es el final del último fragmento, en milisegundos.
func (p Pista) Duracion() int {
	var max int
	for _, e := range p {
		if e.HastaMS > max {
			max = e.HastaMS
		}
	}
	return max
}
