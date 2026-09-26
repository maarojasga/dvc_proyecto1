package httpserver_test

import (
	"bytes"
	"encoding/binary"
	"net/http"
	"testing"

	"github.com/DES-SOLUCIONES-CLOUD/proyecto-1/backend/internal/platform/queue"
)

// wavDePrueba arma un WAV PCM silencioso mínimo. Basta para el olfateo y
// para el reconocimiento de contenedor; no hace falta un archivo real de
// estudio.
func wavDePrueba() []byte {
	const muestras = 256
	dataSize := muestras * 2
	b := make([]byte, 44+dataSize)
	copy(b[0:], []byte("RIFF"))
	binary.LittleEndian.PutUint32(b[4:], uint32(36+dataSize))
	copy(b[8:], []byte("WAVE"))
	copy(b[12:], []byte("fmt "))
	binary.LittleEndian.PutUint32(b[16:], 16)
	binary.LittleEndian.PutUint16(b[20:], 1)
	binary.LittleEndian.PutUint16(b[22:], 1)
	binary.LittleEndian.PutUint32(b[24:], 8000)
	binary.LittleEndian.PutUint32(b[28:], 16000)
	binary.LittleEndian.PutUint16(b[32:], 2)
	binary.LittleEndian.PutUint16(b[34:], 16)
	copy(b[36:], []byte("data"))
	binary.LittleEndian.PutUint32(b[40:], uint32(dataSize))
	return b
}

func (e *entorno) confirmarAudio(c *cliente, versionID, recursoID string, datos []byte) respuesta {
	e.t.Helper()
	base := "/courses/versions/" + versionID + "/resources/" + recursoID
	if res := c.hacer(http.MethodPost, base+"/upload-url",
		map[string]any{"mime_type": "audio/wav"}); res.Estado != http.StatusOK {
		e.t.Fatalf("pedir url de subida: %d %s", res.Estado, res.Crudo)
	}
	e.almacen.ponerObjeto("resources/"+recursoID+"/original", datos, "audio/wav")
	return c.hacer(http.MethodPost, base+"/confirm-upload", map[string]any{})
}

// Un bloque de bytes que no es audio se rechaza al confirmar: encolarlo
// solo produciría el mismo fallo de FFmpeg tres veces.
func TestUnAudioSinContenedorSeRechazaAlSubir(t *testing.T) {
	env := nuevoEntorno(t)
	c, versionID := env.profesorConCurso("prof-basura@example.com", "curso-basura")
	recursoID := recursoParaCarga(t, c, versionID, "audio", "Audio falso")

	res := env.confirmarAudio(c, versionID, recursoID, bytes.Repeat([]byte{0x07}, 2048))
	if res.Estado != http.StatusUnprocessableEntity {
		t.Fatalf("se esperaba 422, llegó %d: %s", res.Estado, res.Crudo)
	}
	if codigo, _ := res.campo(t, "error", "code").(string); codigo != "media_container_unrecognized" {
		t.Errorf("código inesperado: %s", res.Crudo)
	}
	if env.almacen.existe("resources/" + recursoID + "/original") {
		t.Error("el objeto rechazado debería haberse borrado del almacén")
	}
	if len(env.cola.publicados()) != 0 {
		t.Error("no debería encolarse un original ilegible")
	}
}

// Confirmar dos veces el mismo original reutiliza el activo y no publica un
// segundo trabajo. Antes cada confirmación creaba un UUID nuevo y Asynq los
// trataba como transcodificaciones distintas.
func TestConfirmarDosVecesNoDuplicaElActivo(t *testing.T) {
	env := nuevoEntorno(t)
	c, versionID := env.profesorConCurso("prof-idem@example.com", "curso-idem")
	recursoID := recursoParaCarga(t, c, versionID, "audio", "Clase de audio")

	primero := env.confirmarAudio(c, versionID, recursoID, wavDePrueba())
	if primero.Estado != http.StatusAccepted {
		t.Fatalf("primera confirmación: %d %s", primero.Estado, primero.Crudo)
	}
	id1, _ := primero.campo(t, "media_asset_id").(string)

	segundo := env.confirmarAudio(c, versionID, recursoID, wavDePrueba())
	if segundo.Estado != http.StatusAccepted {
		t.Fatalf("segunda confirmación: %d %s", segundo.Estado, segundo.Crudo)
	}
	id2, _ := segundo.campo(t, "media_asset_id").(string)
	if id1 == "" || id1 != id2 {
		t.Errorf("deberían devolver el mismo activo: %q vs %q", id1, id2)
	}

	if n := env.contar(`SELECT count(*) FROM media_assets WHERE resource_id=$1`, recursoID); n != 1 {
		t.Errorf("el recurso debería tener un solo activo, tiene %d", n)
	}
	pubs := env.cola.publicados()
	if len(pubs) != 1 {
		t.Fatalf("se esperaba un solo trabajo, hay %d", len(pubs))
	}
	if pubs[0].Tipo != queue.TaskProcessMedia {
		t.Errorf("se encoló %q", pubs[0].Tipo)
	}
	if pubs[0].ClaveDeIdempotencia != id1 {
		t.Errorf("la clave debería ser el activo, es %q", pubs[0].ClaveDeIdempotencia)
	}
}
