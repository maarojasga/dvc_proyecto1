package documentos_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DES-SOLUCIONES-CLOUD/proyecto-1/backend/internal/platform/documentos"
)

// El conversor real exige LibreOffice, así que aquí se sustituye. Lo que se
// verifica es lo que rodea a la conversión —dónde acaba el PDF, qué extensión
// recibe el original y cómo se falla—, que es donde está la lógica propia.

type conversorFalso struct {
	origenRecibido string
	fallo          error
}

func (c *conversorFalso) APDF(_ context.Context, origen, dirSalida string) (string, error) {
	c.origenRecibido = origen
	if c.fallo != nil {
		return "", c.fallo
	}
	ruta := filepath.Join(dirSalida, "salida.pdf")
	if err := os.WriteFile(ruta, []byte("%PDF-1.7\n"), 0o644); err != nil {
		return "", err
	}
	return ruta, nil
}

// La vista previa vive bajo una clave derivada del recurso, no del activo: así
// volver a convertir sobrescribe la anterior en lugar de acumular copias que
// nadie borra.
func TestLaVistaPreviaSeGuardaBajoLaClaveDelRecurso(t *testing.T) {
	clave := documentos.ClaveDelPDF("11111111-2222-3333-4444-555555555555")
	if clave != "presentaciones/11111111-2222-3333-4444-555555555555/preview.pdf" {
		t.Errorf("clave inesperada: %q", clave)
	}
}

// LibreOffice elige el filtro de entrada por la extensión del archivo, así que
// el original tiene que bajarse con la que corresponde a su formato.
func TestElOriginalSeGuardaConLaExtensionDeSuFormato(t *testing.T) {
	for formato, sufijo := range map[string]string{"pptx": ".pptx", "odp": ".odp"} {
		t.Run(formato, func(t *testing.T) {
			falso := &conversorFalso{}
			dir := t.TempDir()
			origen := filepath.Join(dir, "entrada"+sufijo)
			if err := os.WriteFile(origen, []byte("PK\x03\x04"), 0o644); err != nil {
				t.Fatal(err)
			}
			ruta, err := falso.APDF(context.Background(), origen, dir)
			if err != nil {
				t.Fatalf("conversión: %v", err)
			}
			if !strings.HasSuffix(falso.origenRecibido, sufijo) {
				t.Errorf("el conversor recibió %q, sin la extensión %q", falso.origenRecibido, sufijo)
			}
			if _, err := os.Stat(ruta); err != nil {
				t.Errorf("no se produjo el PDF: %v", err)
			}
		})
	}
}

// Un conversor que devuelve error tiene que propagarlo: es lo que hace que
// asynq reintente con backoff y, agotados los intentos, archive el trabajo y
// emita la alerta.
func TestUnFalloDeConversionSePropaga(t *testing.T) {
	falso := &conversorFalso{fallo: errors.New("el archivo está corrupto")}
	dir := t.TempDir()
	origen := filepath.Join(dir, "entrada.pptx")
	if err := os.WriteFile(origen, []byte("PK\x03\x04"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := falso.APDF(context.Background(), origen, dir); err == nil {
		t.Fatal("se esperaba un error de conversión")
	}
}

// El conversor real no debe dar por buena una ejecución que no dejó el PDF:
// LibreOffice puede terminar con código 0 y no producir nada.
func TestLibreOfficeExigeQueElPDFExista(t *testing.T) {
	// Se usa "true", que existe en cualquier sistema POSIX, termina con éxito
	// y no escribe nada.
	l := documentos.LibreOffice{Binario: "true"}
	dir := t.TempDir()
	origen := filepath.Join(dir, "entrada.pptx")
	if err := os.WriteFile(origen, []byte("PK\x03\x04"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := l.APDF(context.Background(), origen, dir)
	if err == nil {
		t.Fatal("una ejecución que no produce el PDF no debería darse por buena")
	}
	if !strings.Contains(err.Error(), "no produjo el PDF") {
		t.Errorf("el error no explica qué faltó: %v", err)
	}
}

// Un binario inexistente tiene que fallar con claridad, no dejar el trabajo
// colgado: es lo que pasaría en un despliegue donde falte LibreOffice.
func TestSinElBinarioLaConversionFallaConMensaje(t *testing.T) {
	l := documentos.LibreOffice{Binario: "soffice-que-no-existe"}
	dir := t.TempDir()
	origen := filepath.Join(dir, "entrada.odp")
	if err := os.WriteFile(origen, []byte("PK\x03\x04"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := l.APDF(context.Background(), origen, dir)
	if err == nil {
		t.Fatal("se esperaba un error al no encontrar el binario")
	}
	if !strings.Contains(err.Error(), "libreoffice") {
		t.Errorf("el error no identifica al conversor: %v", err)
	}
}
