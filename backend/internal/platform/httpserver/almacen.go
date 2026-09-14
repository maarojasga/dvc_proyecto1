package httpserver

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/DES-SOLUCIONES-CLOUD/proyecto-1/backend/internal/domain/documento"
	"github.com/DES-SOLUCIONES-CLOUD/proyecto-1/backend/internal/platform/antimalware"
	"github.com/DES-SOLUCIONES-CLOUD/proyecto-1/backend/internal/platform/storage"

	domain "github.com/DES-SOLUCIONES-CLOUD/proyecto-1/backend/internal/domain/course"
)

// AlmacenDeObjetos es lo que la autoría necesita del almacenamiento: firmar
// subidas, conducir una carga multipart y releer el objeto para verificarlo.
//
// Se declara como interfaz, y no como *storage.Client, por el mismo motivo que
// EntregaDeObjetos: los manejadores no deben depender del SDK del proveedor, y
// las pruebas del ciclo multipart tienen que poder correr sin MinIO levantado.
type AlmacenDeObjetos interface {
	PresignedPutURL(ctx context.Context, objectKey string, expiry time.Duration) (string, error)
	InitiateMultipartUpload(ctx context.Context, objectKey, contentType string) (string, error)
	PresignedUploadPartURL(ctx context.Context, objectKey, uploadID string, partNumber int, expiry time.Duration) (string, error)
	CompleteMultipartUpload(ctx context.Context, objectKey, uploadID string, partes []storage.ParteCargada) error
	AbortMultipartUpload(ctx context.Context, objectKey, uploadID string) error
	ListObjectParts(ctx context.Context, objectKey, uploadID string) ([]storage.ParteCargada, error)
	StatObject(ctx context.Context, objectKey string) (storage.ObjetoInfo, error)
	AbrirObjeto(ctx context.Context, objectKey string) (io.ReadCloser, error)
	RemoveObject(ctx context.Context, objectKey string) error
}

// vigenciaDeCarga es lo que dura una carga directa al almacén. La especifica
// el alcance mínimo: una subida interrumpida se puede reanudar durante 24
// horas, así que tanto la URL de un PUT simple como las de cada parte de una
// multipart siguen siendo válidas ese tiempo.
const vigenciaDeCarga = 24 * time.Hour

// Fallos de la verificación posterior a la carga. Son del cliente (422) y no
// del servidor: el objeto llegó, pero no es aceptable.
var (
	ErrObjetoVacio        = errors.New("el objeto cargado no existe o está vacío")
	ErrChecksumNoCoincide = errors.New("fallo de integridad: el checksum SHA-256 no coincide con el objeto almacenado")
	ErrArchivoInfectado   = errors.New("el escaneo antimalware rechazó el archivo")
	ErrTipoNoCorresponde  = errors.New("el contenido real del archivo no corresponde al tipo del recurso")
)

// objetoVerificado es lo que se sabe del objeto después de comprobarlo.
type objetoVerificado struct {
	Tamano int64
	// MIME es el tipo deducido de los primeros bytes, no el que declaró el
	// cliente: lo declarado no acredita nada.
	MIME   string
	SHA256 string
	// Formato solo se rellena en presentaciones: distingue PPTX de ODP, que
	// comparten contenedor ZIP y por tanto el mismo MIME. Lo necesita el
	// worker para elegir el filtro de entrada del convertidor.
	Formato documento.Formato
}

// verificarCarga comprueba un objeto recién subido antes de aceptarlo como
// material del curso, en una sola lectura del almacén.
//
// Hace las tres comprobaciones que pide el alcance mínimo —integridad, MIME
// real y escaneo antimalware— y es el único camino por el que un binario entra
// a la plataforma: la subida simple y la multipart terminan las dos aquí. Esa
// unificación es deliberada, porque antes la subida simple no comprobaba nada
// y bastaba con mandar un archivo pequeño para saltarse todo el control.
//
// Cuando algo falla, el objeto se borra del almacén: dejarlo ahí conserva un
// archivo rechazado bajo una clave que el recurso ya conoce.
func (h *handlers) verificarCarga(ctx context.Context, objectKey, checksumDeclarado string, tipo domain.ResourceType) (objetoVerificado, error) {
	info, err := h.deps.Storage.StatObject(ctx, objectKey)
	if err != nil || info.Tamano == 0 {
		return objetoVerificado{}, ErrObjetoVacio
	}

	obj, err := h.deps.Storage.AbrirObjeto(ctx, objectKey)
	if err != nil {
		return objetoVerificado{}, err
	}
	defer obj.Close()

	// Una sola pasada alimenta a la vez el hash, el olfateo de MIME y el
	// escáner: el objeto puede pesar gigabytes y antes se descargaba entero
	// una vez por comprobación.
	hash := sha256.New()
	cabecera := &primerosBytes{limite: 512}
	escaner := h.escaner()
	veredicto, err := escaner.Escanear(ctx, io.TeeReader(obj, io.MultiWriter(hash, cabecera)))
	if err != nil {
		// Un escáner caído no vuelve limpio el objeto: se rechaza la carga.
		return objetoVerificado{}, err
	}

	res := objetoVerificado{
		Tamano: info.Tamano,
		SHA256: hex.EncodeToString(hash.Sum(nil)),
		MIME:   http.DetectContentType(cabecera.datos),
	}
	if tipo == domain.ResourcePresentation {
		res.Formato = documento.Detectar(cabecera.datos)
	}

	if !veredicto.Limpio {
		_ = h.deps.Storage.RemoveObject(ctx, objectKey)
		return objetoVerificado{}, fmt.Errorf("%w: %s", ErrArchivoInfectado, veredicto.Firma)
	}
	if checksumDeclarado != "" && !strings.EqualFold(checksumDeclarado, res.SHA256) {
		_ = h.deps.Storage.RemoveObject(ctx, objectKey)
		return objetoVerificado{}, ErrChecksumNoCoincide
	}
	if !mimeCorresponde(tipo, res.MIME) {
		_ = h.deps.Storage.RemoveObject(ctx, objectKey)
		return objetoVerificado{}, fmt.Errorf("%w: se detectó %s", ErrTipoNoCorresponde, res.MIME)
	}
	// Una presentación que no se sepa convertir se rechaza aquí y no en el
	// worker: fallar al subir dice con claridad qué pasó, mientras que fallar
	// una cola más adelante deja el recurso en "failed" sin explicación útil
	// para quien lo subió.
	if tipo == domain.ResourcePresentation && res.Formato == documento.Desconocido {
		_ = h.deps.Storage.RemoveObject(ctx, objectKey)
		return objetoVerificado{}, documento.ErrFormatoNoSoportado
	}
	return res, nil
}

// escaner devuelve el escáner configurado, o el integrado si el despliegue no
// inyectó ninguno.
func (h *handlers) escaner() antimalware.Escaner {
	if h.deps.Antimalware != nil {
		return h.deps.Antimalware
	}
	return antimalware.Integrado{}
}

// mimeCorresponde contrasta el tipo real del contenido con el tipo declarado
// del recurso.
//
// Es tolerante a propósito con application/octet-stream: los contenedores de
// vídeo y audio que el olfateo de la biblioteca estándar no reconoce (mkv,
// buena parte de los mov) caen ahí, y rechazarlos rompería cargas legítimas.
// Lo que sí corta es la discrepancia clara: un HTML subido como vídeo, o un
// recurso PDF cuyos bytes no empiezan por %PDF.
func mimeCorresponde(tipo domain.ResourceType, mime string) bool {
	familia, _, _ := strings.Cut(mime, "/")
	generico := mime == "application/octet-stream"

	switch tipo {
	case domain.ResourcePDF:
		return mime == "application/pdf"
	case domain.ResourceImage:
		return familia == "image"
	case domain.ResourceVideo:
		return familia == "video" || generico
	case domain.ResourceAudio:
		// Varios contenedores de audio (m4a, algunos ogg) se olfatean como
		// vídeo porque comparten la envoltura.
		return familia == "audio" || familia == "video" || generico
	case domain.ResourcePresentation:
		// PPTX y ODP son contenedores ZIP, así que el olfateo genérico los da
		// por application/zip. Distinguirlos es cosa de documento.Detectar,
		// que mira el nombre de la primera entrada del contenedor.
		//
		// Los formatos heredados (.ppt) no entran: son contenedores OLE, y el
		// escáner antimalware los rechaza antes de llegar aquí por admitir
		// macros. Convertirlos exigiría confiar en un formato que es un vector
		// conocido, y el alcance pide PPTX y ODP.
		return mime == "application/zip" || generico
	default:
		// Los descargables admiten cualquier formato que haya superado el
		// escaneo: ahí el control es el antimalware, no el MIME.
		return true
	}
}

// primerosBytes conserva el principio del flujo mientras este se copia, para
// olfatear el MIME sin volver a leer el objeto.
type primerosBytes struct {
	limite int
	datos  []byte
}

func (p *primerosBytes) Write(b []byte) (int, error) {
	if falta := p.limite - len(p.datos); falta > 0 {
		if len(b) < falta {
			falta = len(b)
		}
		p.datos = append(p.datos, b[:falta]...)
	}
	return len(b), nil
}
