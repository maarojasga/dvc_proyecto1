package httpserver_test

import (
	"bytes"
	"encoding/binary"
	"net/http"
	"testing"

	"github.com/DES-SOLUCIONES-CLOUD/proyecto-1/backend/internal/platform/queue"
)

// Cubren el alcance opcional 5.2: conversión de PPTX y ODP a PDF con
// previsualización. Lo que se verifica aquí es la ingesta —qué se acepta como
// presentación y qué se encola—, no la conversión, que exige LibreOffice y
// tiene sus propias pruebas en internal/platform/documentos.

// presentacion arma el principio de un contenedor ZIP que se reconoce como el
// formato pedido. No hace falta un archivo real: el reconocimiento ocurre
// sobre los primeros bytes.
func presentacion(formato string) []byte {
	nombre, contenido := "[Content_Types].xml", "<?xml version=\"1.0\"?>"
	if formato == "odp" {
		nombre, contenido = "mimetype", "application/vnd.oasis.opendocument.presentation"
	}
	var b bytes.Buffer
	b.Write([]byte{'P', 'K', 0x03, 0x04})
	b.Write(make([]byte, 22))
	_ = binary.Write(&b, binary.LittleEndian, uint16(len(nombre)))
	_ = binary.Write(&b, binary.LittleEndian, uint16(0))
	b.WriteString(nombre)
	b.WriteString(contenido)
	b.Write(bytes.Repeat([]byte("x"), 256)) // relleno, para que no esté vacío
	return b.Bytes()
}

// subirPresentacion crea un recurso de presentación y confirma su carga.
func (e *entorno) subirPresentacion(c *cliente, versionID string, datos []byte) respuesta {
	e.t.Helper()
	recursoID := recursoParaCarga(e.t, c, versionID, "presentation", "Diapositivas")
	base := "/courses/versions/" + versionID + "/resources/" + recursoID
	clave := "resources/" + recursoID + "/original"

	if res := c.hacer(http.MethodPost, base+"/upload-url",
		map[string]any{"mime_type": "application/vnd.openxmlformats-officedocument.presentationml.presentation"}); res.Estado != http.StatusOK {
		e.t.Fatalf("pedir url de subida: %d %s", res.Estado, res.Crudo)
	}
	e.almacen.ponerObjeto(clave, datos, "application/octet-stream")
	return c.hacer(http.MethodPost, base+"/confirm-upload", map[string]any{})
}

// Una presentación válida no queda lista de inmediato: se encola su conversión,
// igual que un vídeo se encola para transcodificar.
func TestUnaPresentacionSeEncolaParaConvertirse(t *testing.T) {
	for _, formato := range []string{"pptx", "odp"} {
		t.Run(formato, func(t *testing.T) {
			env := nuevoEntorno(t)
			c, versionID := env.profesorConCurso("prof-ppt@example.com", "curso-ppt")

			res := env.subirPresentacion(c, versionID, presentacion(formato))
			if res.Estado != http.StatusAccepted {
				t.Fatalf("se esperaba 202 (encolado), llegó %d: %s", res.Estado, res.Crudo)
			}
			if estado, _ := res.campo(t, "status").(string); estado != "queued" {
				t.Errorf("estado inesperado: %s", res.Crudo)
			}

			// El 202 es idéntico al de un vídeo, así que lo que distingue una
			// conversión de una transcodificación solo se ve en la cola.
			trabajo := env.cola.ultimo()
			if trabajo.Tipo != queue.TaskConvertDocument {
				t.Fatalf("se encoló %q en vez de una conversión", trabajo.Tipo)
			}
			carga, ok := trabajo.Payload.(queue.DocumentConvertPayload)
			if !ok {
				t.Fatalf("payload inesperado: %T", trabajo.Payload)
			}
			if carga.Formato != formato {
				t.Errorf("el worker recibiría el formato %q en vez de %q", carga.Formato, formato)
			}
			if trabajo.ClaveDeIdempotencia == "" {
				t.Error("el trabajo se encoló sin clave de idempotencia")
			}
		})
	}
}

// Un vídeo sigue yendo a transcodificar: el cambio de las presentaciones no
// debe haber desviado el resto.
func TestUnVideoSigueYendoATranscodificarse(t *testing.T) {
	env := nuevoEntorno(t)
	c, versionID := env.profesorConCurso("prof-video@example.com", "curso-video")

	recursoID := recursoParaCarga(t, c, versionID, "video", "Clase")
	base := "/courses/versions/" + versionID + "/resources/" + recursoID
	if res := c.hacer(http.MethodPost, base+"/upload-url", map[string]any{"mime_type": "video/mp4"}); res.Estado != http.StatusOK {
		t.Fatalf("url de subida: %d %s", res.Estado, res.Crudo)
	}
	// Una cabecera de MP4 basta: lo que se prueba es el encolado.
	mp4 := append([]byte{0, 0, 0, 0x20}, []byte("ftypisom")...)
	env.almacen.ponerObjeto("resources/"+recursoID+"/original", append(mp4, bytes.Repeat([]byte("x"), 256)...), "video/mp4")

	if res := c.hacer(http.MethodPost, base+"/confirm-upload", map[string]any{}); res.Estado != http.StatusAccepted {
		t.Fatalf("confirmar: %d %s", res.Estado, res.Crudo)
	}
	if tipo := env.cola.ultimo().Tipo; tipo != queue.TaskProcessMedia {
		t.Errorf("se encoló %q en vez de una transcodificación", tipo)
	}
}

// Un ZIP que no es una presentación se rechaza al subirlo y no una cola más
// adelante: fallar aquí dice con claridad qué pasó.
func TestUnZipQueNoEsPresentacionSeRechazaAlSubir(t *testing.T) {
	env := nuevoEntorno(t)
	c, versionID := env.profesorConCurso("prof-zip@example.com", "curso-zip")

	var b bytes.Buffer
	b.Write([]byte{'P', 'K', 0x03, 0x04})
	b.Write(make([]byte, 22))
	_ = binary.Write(&b, binary.LittleEndian, uint16(len("fotos/playa.jpg")))
	_ = binary.Write(&b, binary.LittleEndian, uint16(0))
	b.WriteString("fotos/playa.jpg")
	b.Write(bytes.Repeat([]byte("x"), 256))

	res := env.subirPresentacion(c, versionID, b.Bytes())
	if res.Estado != http.StatusUnprocessableEntity {
		t.Fatalf("se esperaba 422, llegó %d: %s", res.Estado, res.Crudo)
	}
	if codigo, _ := res.campo(t, "error", "code").(string); codigo != "presentation_format_unsupported" {
		t.Errorf("código inesperado: %s", res.Crudo)
	}
}

// Los formatos heredados de Office son contenedores OLE, que admiten macros.
// El escaneo los corta antes de llegar al reconocimiento de formato.
func TestUnPptHeredadoLoRechazaElAntimalware(t *testing.T) {
	env := nuevoEntorno(t)
	c, versionID := env.profesorConCurso("prof-ole@example.com", "curso-ole")

	ole := append([]byte{0xD0, 0xCF, 0x11, 0xE0, 0xA1, 0xB1, 0x1A, 0xE1}, bytes.Repeat([]byte{0x00}, 256)...)
	res := env.subirPresentacion(c, versionID, ole)
	if res.Estado != http.StatusUnprocessableEntity {
		t.Fatalf("se esperaba 422, llegó %d: %s", res.Estado, res.Crudo)
	}
	if codigo, _ := res.campo(t, "error", "code").(string); codigo != "malware_detected" {
		t.Errorf("código inesperado: %s", res.Crudo)
	}
}

// Mientras la conversión no termine, el recurso no está listo y por tanto la
// versión no se puede publicar: publicarla pondría delante del estudiante algo
// que todavía no se puede ver.
func TestUnaPresentacionSinConvertirBloqueaLaPublicacion(t *testing.T) {
	env := nuevoEntorno(t)
	c, versionID := env.profesorConCurso("prof-pub@example.com", "curso-pub")

	if res := env.subirPresentacion(c, versionID, presentacion("pptx")); res.Estado != http.StatusAccepted {
		t.Fatalf("subir presentación: %d %s", res.Estado, res.Crudo)
	}
	if res := c.hacer(http.MethodPatch, "/courses/versions/"+versionID,
		map[string]any{"title": "Curso", "summary": "Resumen"}); res.Estado != http.StatusOK {
		t.Fatalf("metadatos: %d %s", res.Estado, res.Crudo)
	}

	res := c.hacer(http.MethodPost, "/courses/versions/"+versionID+"/publish", nil)
	if res.Estado != http.StatusUnprocessableEntity {
		t.Fatalf("se esperaba 422, llegó %d: %s", res.Estado, res.Crudo)
	}
	if !contiene(res.Crudo, "procesar") {
		t.Errorf("el motivo no menciona el procesamiento pendiente: %s", res.Crudo)
	}
}

// Ya convertida, lo que se entrega es el PDF, que es lo que el visor sabe
// mostrar; la presentación original no la abre el navegador.
func TestLaPresentacionConvertidaSeEntregaComoPDF(t *testing.T) {
	env := nuevoEntorno(t)
	c, versionID := env.profesorConCurso("prof-conv@example.com", "curso-conv")

	recursoID := recursoParaCarga(t, c, versionID, "presentation", "Diapositivas")
	base := "/courses/versions/" + versionID + "/resources/" + recursoID
	clave := "resources/" + recursoID + "/original"
	if res := c.hacer(http.MethodPost, base+"/upload-url", map[string]any{"mime_type": "application/zip"}); res.Estado != http.StatusOK {
		t.Fatalf("url de subida: %d %s", res.Estado, res.Crudo)
	}
	env.almacen.ponerObjeto(clave, presentacion("pptx"), "application/octet-stream")
	if res := c.hacer(http.MethodPost, base+"/confirm-upload", map[string]any{}); res.Estado != http.StatusAccepted {
		t.Fatalf("confirmar: %d %s", res.Estado, res.Crudo)
	}

	// Se simula el final del worker: la conversión dejó el PDF y marcó listo.
	env.marcarPresentacionConvertida(recursoID)

	if res := c.hacer(http.MethodPatch, "/courses/versions/"+versionID,
		map[string]any{"title": "Curso", "summary": "Resumen"}); res.Estado != http.StatusOK {
		t.Fatalf("metadatos: %d %s", res.Estado, res.Crudo)
	}
	if res := c.hacer(http.MethodPost, "/courses/versions/"+versionID+"/publish", nil); res.Estado != http.StatusOK {
		t.Fatalf("publicar: %d %s", res.Estado, res.Crudo)
	}

	res := c.hacer(http.MethodGet, "/resources/"+recursoID+"/content", nil)
	if res.Estado != http.StatusOK {
		t.Fatalf("contenido: %d %s", res.Estado, res.Crudo)
	}
	url, _ := res.campo(t, "url").(string)
	if !contiene(url, "presentaciones/"+recursoID+"/preview.pdf") {
		t.Errorf("no se entrega el PDF convertido: %s", res.Crudo)
	}
}
