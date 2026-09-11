package httpserver

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"

	"github.com/DES-SOLUCIONES-CLOUD/proyecto-1/backend/internal/platform/antimalware"
	"github.com/DES-SOLUCIONES-CLOUD/proyecto-1/backend/internal/platform/storage"
)

// Hay dos caminos para subir material —un PUT prefirmado y una carga multipart
// reanudable— y los dos tienen que aplicar las mismas comprobaciones. Vivían
// duplicadas y habían empezado a divergir: una exigía el checksum y la otra
// no. Aquí van juntas, una sola vez.

// tamanoMinimoDeCarga descarta el objeto vacío, que es lo que queda cuando una
// subida se cortó antes de empezar.
const tamanoMinimoDeCarga = 1

// ErrChecksumRequerido se devuelve cuando el cliente no declara el SHA-256 de
// lo que subió. Se exige y no se acepta omitido: un checksum opcional no
// verifica integridad, solo la documenta cuando ya salió bien.
var ErrChecksumRequerido = errors.New("falta checksum_sha256: hay que declarar el SHA-256 del archivo subido")

// ErrIntegridad indica que lo que llegó al almacén no es lo que el cliente dijo
// que subía.
var ErrIntegridad = errors.New("fallo de integridad: el SHA-256 del objeto no coincide con el declarado")

// mimesProhibidos son formatos que no tienen ninguna razón para ser material
// de un curso y sí muchas para ser un problema si un estudiante los descarga.
var mimesProhibidos = map[string]bool{
	"application/x-dosexec":     true,
	"application/x-executable":  true,
	"application/x-sharedlib":   true,
	"application/x-mach-binary": true,
	// HTML es el caso menos obvio y el más peligroso: si se sirve en línea
	// desde el dominio del almacén, ejecuta scripts con ese origen.
	"text/html": true,
}

// cargaVerificada es lo que se supo del objeto al comprobarlo.
type cargaVerificada struct {
	Info     storage.ObjetoInfo
	MIME     string // el detectado de los bytes, no el declarado por el cliente
	Checksum string
}

// verificarCarga comprueba que el objeto existe, que su contenido coincide con
// el checksum declarado, que su MIME real no es de los prohibidos y que pasa
// el antimalware. Si algo falla, borra el objeto: dejarlo sería guardar
// material que ya se decidió rechazar.
//
// El orden importa. Primero lo barato (existe, tamaño), luego la integridad
// —porque escanear un objeto corrupto no dice nada útil—, luego el MIME real y
// solo al final el escaneo, que es lo más caro.
func (h *handlers) verificarCarga(ctx context.Context, objectKey, checksumDeclarado string) (cargaVerificada, error) {
	if checksumDeclarado == "" {
		return cargaVerificada{}, ErrChecksumRequerido
	}

	info, err := h.deps.Storage.Metadatos(ctx, objectKey)
	if err != nil || info.Tamano < tamanoMinimoDeCarga {
		return cargaVerificada{}, errors.New("el objeto cargado no existe o está vacío")
	}

	calculado, err := h.deps.Storage.CalculateSHA256(ctx, objectKey)
	if err != nil {
		return cargaVerificada{}, fmt.Errorf("no se pudo verificar la integridad: %w", err)
	}
	if calculado != checksumDeclarado {
		h.descartar(ctx, objectKey)
		return cargaVerificada{}, ErrIntegridad
	}

	mime, err := h.deps.Storage.DetectMIME(ctx, objectKey)
	if err != nil || mime == "" {
		mime = info.ContentType
	}
	// El MIME real puede venir con parámetros ("text/html; charset=utf-8").
	base := mime
	if i := indiceDePuntoYComa(base); i >= 0 {
		base = base[:i]
	}
	if mimesProhibidos[base] {
		h.descartar(ctx, objectKey)
		return cargaVerificada{}, fmt.Errorf("%w: el contenido real es %s, que no se admite como material de curso", ErrTipoNoAdmitido, base)
	}

	cuerpo, err := h.deps.Storage.AbrirObjeto(ctx, objectKey)
	if err != nil {
		return cargaVerificada{}, fmt.Errorf("no se pudo leer el objeto para escanearlo: %w", err)
	}
	veredicto, err := h.deps.Antimalware.Escanear(ctx, cuerpo)
	_ = cuerpo.Close()
	if err != nil {
		// No se pudo escanear no es estar limpio. El objeto se queda (puede
		// reintentarse) pero el recurso no avanza.
		return cargaVerificada{}, fmt.Errorf("no se pudo completar el escaneo antimalware: %w", err)
	}
	if !veredicto.Limpio {
		h.descartar(ctx, objectKey)
		return cargaVerificada{}, fmt.Errorf("%w (%s)", antimalware.ErrInfectado, veredicto.Firma)
	}

	return cargaVerificada{Info: info, MIME: base, Checksum: calculado}, nil
}

// ErrTipoNoAdmitido marca un MIME real que no se acepta.
var ErrTipoNoAdmitido = errors.New("tipo de archivo no admitido")

// descartar borra el objeto rechazado. El error se ignora a propósito: la
// respuesta al profesor es el rechazo, y no poder limpiar no lo cambia.
func (h *handlers) descartar(ctx context.Context, objectKey string) {
	_ = h.deps.Storage.RemoveObject(ctx, objectKey)
}

func indiceDePuntoYComa(s string) int {
	for i := 0; i < len(s); i++ {
		if s[i] == ';' {
			return i
		}
	}
	return -1
}

// responderCargaRechazada traduce el rechazo al código HTTP y al código de
// error que le corresponde, para que el profesor vea 422 con un motivo y no un
// 500 genérico, y lo deja en el log: un archivo rechazado por el antimalware es
// justo lo que alguien querrá encontrar después.
func (h *handlers) responderCargaRechazada(w http.ResponseWriter, r *http.Request, objectKey string, err error) {
	estado, codigo := http.StatusUnprocessableEntity, "carga_rechazada"
	switch {
	case errors.Is(err, ErrChecksumRequerido):
		estado, codigo = http.StatusBadRequest, "checksum_requerido"
	case errors.Is(err, ErrIntegridad):
		codigo = "integridad_fallida"
	case errors.Is(err, ErrTipoNoAdmitido):
		codigo = "tipo_no_admitido"
	case errors.Is(err, antimalware.ErrInfectado):
		codigo = "archivo_infectado"
	default:
		// No es un rechazo, es un fallo: que lo clasifique el camino normal.
		writeError(w, err)
		return
	}
	log.Printf("request_id=%s carga rechazada object_key=%s motivo=%s detalle=%q",
		requestIDFromContext(r.Context()), objectKey, codigo, err.Error())
	writeJSON(w, estado, errorEnvelope{Error: errorBody{Code: codigo, Message: err.Error()}})
}
