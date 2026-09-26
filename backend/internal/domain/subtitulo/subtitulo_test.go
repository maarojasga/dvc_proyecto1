package subtitulo_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/DES-SOLUCIONES-CLOUD/proyecto-1/backend/internal/domain/subtitulo"
)

const pistaDePrueba = `WEBVTT

00:00:00.000 --> 00:00:03.500
Bienvenidos al curso

00:00:03.500 --> 00:00:07.000
Hoy hablaremos de
almacenamiento de objetos
`

func TestAnalizaUnaPistaCorriente(t *testing.T) {
	p, err := subtitulo.Analizar(pistaDePrueba)
	if err != nil {
		t.Fatalf("analizar: %v", err)
	}
	if len(p) != 2 {
		t.Fatalf("se esperaban 2 fragmentos, hay %d: %+v", len(p), p)
	}
	if p[0].DesdeMS != 0 || p[0].HastaMS != 3500 {
		t.Errorf("tiempos del primer fragmento: %+v", p[0])
	}
	if p[1].Texto != "Hoy hablaremos de\nalmacenamiento de objetos" {
		t.Errorf("texto del segundo fragmento: %q", p[1].Texto)
	}
}

// Un SRT es casi idéntico pero el navegador lo rechaza. Aceptarlo aquí solo
// trasladaría el fallo a la pantalla del estudiante.
func TestUnSRTNoPasaPorWebVTT(t *testing.T) {
	srt := "1\n00:00:00,000 --> 00:00:03,500\nHola\n"
	if _, err := subtitulo.Analizar(srt); !errors.Is(err, subtitulo.ErrNoEsWebVTT) {
		t.Fatalf("se esperaba ErrNoEsWebVTT, llegó %v", err)
	}
}

func TestAceptaTiemposSinHoras(t *testing.T) {
	p, err := subtitulo.Analizar("WEBVTT\n\n01:02.500 --> 01:05.000\nTexto\n")
	if err != nil {
		t.Fatalf("analizar: %v", err)
	}
	if len(p) != 1 || p[0].DesdeMS != 62500 {
		t.Errorf("tiempos sin horas mal interpretados: %+v", p)
	}
}

// ".5" es medio segundo, no cinco milésimas: interpretarlo mal desplazaría
// los subtítulos casi un segundo.
func TestUnaFraccionCortaEsDecimalYNoMilesimas(t *testing.T) {
	p, _ := subtitulo.Analizar("WEBVTT\n\n00:00:01.5 --> 00:00:02.25\nTexto\n")
	if len(p) != 1 {
		t.Fatalf("no se analizó el fragmento: %+v", p)
	}
	if p[0].DesdeMS != 1500 || p[0].HastaMS != 2250 {
		t.Errorf("fracciones mal interpretadas: %+v", p[0])
	}
}

// Una pista real trae identificadores, notas y estilos entre los fragmentos.
// Rechazarla entera por eso dejaría el vídeo sin subtítulos.
func TestIgnoraNotasIdentificadoresYEstilos(t *testing.T) {
	crudo := `WEBVTT - Curso de nubes

NOTE
Esta pista la generó un servicio automático.

STYLE
::cue { color: yellow }

fragmento-1
00:00:00.000 --> 00:00:02.000
Primero

00:00:02.000 --> 00:00:04.000 align:start line:90%
Segundo
`
	p, err := subtitulo.Analizar(crudo)
	if err != nil {
		t.Fatalf("analizar: %v", err)
	}
	if len(p) != 2 {
		t.Fatalf("se esperaban 2 fragmentos, hay %d: %+v", len(p), p)
	}
	if p[0].Texto != "Primero" || p[1].Texto != "Segundo" {
		t.Errorf("se coló contenido que no era de los fragmentos: %+v", p)
	}
}

func TestLaTranscripcionUneLosFragmentosEnTextoCorrido(t *testing.T) {
	p, _ := subtitulo.Analizar(pistaDePrueba)
	got := p.Transcripcion()
	quiero := "Bienvenidos al curso Hoy hablaremos de almacenamiento de objetos"
	if got != quiero {
		t.Errorf("transcripción:\n  got:  %q\n  want: %q", got, quiero)
	}
}

// El marcado de WebVTT es para pintar el subtítulo, no para leerlo.
func TestLaTranscripcionQuitaElMarcado(t *testing.T) {
	crudo := "WEBVTT\n\n00:00:00.000 --> 00:00:02.000\n<v Ana>Hola <i>a todos</i><00:00:01.000> ya\n"
	p, _ := subtitulo.Analizar(crudo)
	if got := p.Transcripcion(); got != "Hola a todos ya" {
		t.Errorf("transcripción con marcado: %q", got)
	}
}

func TestLaDuracionEsElFinalDelUltimoFragmento(t *testing.T) {
	p, _ := subtitulo.Analizar(pistaDePrueba)
	if p.Duracion() != 7000 {
		t.Errorf("duración %d, se esperaba 7000", p.Duracion())
	}
}

func TestUnaPistaSinFragmentosNoEsUnError(t *testing.T) {
	// Una pista vacía es válida: significa que todavía no hay subtítulos.
	p, err := subtitulo.Analizar("WEBVTT\n\n")
	if err != nil {
		t.Fatalf("analizar: %v", err)
	}
	if len(p) != 0 || p.Transcripcion() != "" {
		t.Errorf("se inventó contenido: %+v", p)
	}
}

func TestUnBOMAlPrincipioNoRompeElReconocimiento(t *testing.T) {
	// Varios editores guardan el archivo con marca de orden de bytes.
	con := "\uFEFF" + pistaDePrueba
	if _, err := subtitulo.Analizar(con); err != nil {
		t.Errorf("un BOM no debería impedir reconocer el formato: %v", err)
	}
}

func TestUnFragmentoConTiemposIlegiblesSeDescartaSinTumbarLaPista(t *testing.T) {
	crudo := strings.Join([]string{
		"WEBVTT", "",
		"no-son-tiempos --> tampoco", "Se descarta", "",
		"00:00:05.000 --> 00:00:06.000", "Este sí", "",
	}, "\n")
	p, err := subtitulo.Analizar(crudo)
	if err != nil {
		t.Fatalf("analizar: %v", err)
	}
	if len(p) != 1 || p[0].Texto != "Este sí" {
		t.Errorf("una línea de tiempos ilegible no debería llevarse la pista: %+v", p)
	}
}
