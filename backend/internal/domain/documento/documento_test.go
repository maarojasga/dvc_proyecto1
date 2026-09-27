package documento_test

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/DES-SOLUCIONES-CLOUD/proyecto-1/backend/internal/domain/documento"
)

// cabeceraZIP arma el principio de un contenedor ZIP cuya primera entrada se
// llama como se indique y va seguida del contenido dado.
//
// Se construye a mano en vez de generar un ZIP real porque lo que se prueba es
// justamente el reconocimiento sobre los primeros bytes, sin descomprimir.
func cabeceraZIP(nombre, contenido string) []byte {
	var b bytes.Buffer
	b.Write([]byte{'P', 'K', 0x03, 0x04})
	b.Write(make([]byte, 22)) // versión, banderas, método, fecha, CRC y tamaños
	_ = binary.Write(&b, binary.LittleEndian, uint16(len(nombre)))
	_ = binary.Write(&b, binary.LittleEndian, uint16(0)) // sin campo extra
	b.WriteString(nombre)
	b.WriteString(contenido)
	return b.Bytes()
}

func TestReconoceUnaPresentacionODP(t *testing.T) {
	datos := cabeceraZIP("mimetype", "application/vnd.oasis.opendocument.presentation")
	if f := documento.Detectar(datos); f != documento.ODP {
		t.Errorf("se esperaba odp, llegó %q", f)
	}
}

func TestReconoceUnaPresentacionPPTX(t *testing.T) {
	datos := cabeceraZIP("[Content_Types].xml", "<?xml version=\"1.0\"?>")
	if f := documento.Detectar(datos); f != documento.PPTX {
		t.Errorf("se esperaba pptx, llegó %q", f)
	}
}

// Un texto y una hoja de cálculo de OpenDocument comparten contenedor con la
// presentación: si no se mirara el tipo, entrarían como presentación y la
// conversión fallaría más tarde, con un error mucho menos claro.
func TestOtrosDocumentosOpenDocumentNoPasanPorPresentacion(t *testing.T) {
	casos := map[string]string{
		"texto":           "application/vnd.oasis.opendocument.text",
		"hoja de cálculo": "application/vnd.oasis.opendocument.spreadsheet",
	}
	for nombre, tipo := range casos {
		if f := documento.Detectar(cabeceraZIP("mimetype", tipo)); f != documento.Desconocido {
			t.Errorf("%s se reconoció como %q", nombre, f)
		}
	}
}

func TestUnZipCualquieraNoEsUnaPresentacion(t *testing.T) {
	datos := cabeceraZIP("fotos/playa.jpg", "\xff\xd8\xff")
	if f := documento.Detectar(datos); f != documento.Desconocido {
		t.Errorf("un zip cualquiera se reconoció como %q", f)
	}
}

func TestLoQueNiSiquieraEsZipSeDescarta(t *testing.T) {
	casos := map[string][]byte{
		"PDF":    []byte("%PDF-1.7\n"),
		"vacío":  {},
		"corto":  {'P', 'K'},
		"basura": bytes.Repeat([]byte{0x41}, 100),
	}
	for nombre, datos := range casos {
		if f := documento.Detectar(datos); f != documento.Desconocido {
			t.Errorf("%s se reconoció como %q", nombre, f)
		}
	}
}

// Un largo de nombre que se sale del buffer no debe provocar un pánico: el
// archivo lo sube un usuario y puede venir manipulado a propósito.
func TestUnLargoDeNombreImposibleNoRompe(t *testing.T) {
	datos := cabeceraZIP("mimetype", "application/vnd.oasis.opendocument.presentation")
	binary.LittleEndian.PutUint16(datos[26:28], 60000)
	if f := documento.Detectar(datos); f != documento.Desconocido {
		t.Errorf("se esperaba desconocido, llegó %q", f)
	}
}

func TestCadaFormatoTraeLaExtensionQueEsperaElConvertidor(t *testing.T) {
	if documento.PPTX.Extension() != ".pptx" || documento.ODP.Extension() != ".odp" {
		t.Error("las extensiones no corresponden a los formatos")
	}
	if documento.Desconocido.Extension() != "" {
		t.Error("un formato desconocido no debería proponer extensión")
	}
}
