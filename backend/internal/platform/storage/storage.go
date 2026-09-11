// Package storage adapta el almacenamiento de objetos (S3/MinIO) para
// originales multimedia, derivados HLS, PDFs e imágenes de insignias.
// Ningún binario se persiste en PostgreSQL: la API solo emite URLs
// prefirmadas de subida/descarga.
package storage

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

type Client struct {
	mc     *minio.Client
	bucket string

	// firmante firma las URLs que va a abrir el navegador. Normalmente es
	// el mismo cliente que mc, pero cuando la API habla con el almacén por
	// un nombre de red interno ("minio:9000") y el navegador lo alcanza por
	// otro ("localhost:9100"), son dos clientes distintos: la firma SigV4
	// incluye el Host, así que una URL firmada contra el host interno es
	// inservible fuera de la red de contenedores y no se puede reescribir
	// a posteriori sin invalidar la firma.
	firmante *minio.Client

	publicURL string // si está vacío, se usan URLs prefirmadas también para GET
}

type Config struct {
	Endpoint  string
	AccessKey string
	SecretKey string
	UseSSL    bool
	Bucket    string
	Region    string
	PublicURL string

	// PublicEndpoint es el host por el que el navegador alcanza el almacén.
	// Vacío significa "el mismo que Endpoint".
	PublicEndpoint string
	PublicUseSSL   bool
}

func New(ctx context.Context, cfg Config) (*Client, error) {
	mc, err := minio.New(cfg.Endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""),
		Secure: cfg.UseSSL,
	})
	if err != nil {
		return nil, fmt.Errorf("storage: no se pudo crear el cliente: %w", err)
	}

	exists, err := mc.BucketExists(ctx, cfg.Bucket)
	if err != nil {
		return nil, fmt.Errorf("storage: no se pudo verificar el bucket: %w", err)
	}
	if !exists {
		if err := mc.MakeBucket(ctx, cfg.Bucket, minio.MakeBucketOptions{}); err != nil {
			return nil, fmt.Errorf("storage: no se pudo crear el bucket: %w", err)
		}
	}

	firmante, err := clienteDeFirma(cfg, mc)
	if err != nil {
		return nil, err
	}

	return &Client{mc: mc, firmante: firmante, bucket: cfg.Bucket, publicURL: cfg.PublicURL}, nil
}

// clienteDeFirma devuelve el cliente con el que se firman las URLs que abrirá
// el navegador. Si no hay endpoint público configurado reutiliza el interno,
// que es lo correcto cuando API y navegador ven el almacén por el mismo host.
func clienteDeFirma(cfg Config, interno *minio.Client) (*minio.Client, error) {
	if cfg.PublicEndpoint == "" || cfg.PublicEndpoint == cfg.Endpoint {
		return interno, nil
	}
	// Region va explícita a propósito: sin ella minio-go resuelve la
	// ubicación del bucket con una petición real, y este cliente apunta a un
	// host que puede no resolver desde aquí (es el del navegador). Firmar no
	// debe requerir red.
	region := cfg.Region
	if region == "" {
		region = "us-east-1"
	}
	c, err := minio.New(cfg.PublicEndpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""),
		Secure: cfg.PublicUseSSL,
		Region: region,
	})
	if err != nil {
		return nil, fmt.Errorf("storage: no se pudo crear el cliente público: %w", err)
	}
	return c, nil
}

// SirveDesdeCDN informa si los objetos se entregan por una base pública en
// lugar de firmarse uno a uno. Lo expone la API para que el cliente sepa si
// la URL que recibe caduca.
func (c *Client) SirveDesdeCDN() bool { return c.publicURL != "" }

// PresignedPutURL emite una URL prefirmada de subida directa (carga
// multipart directa a objetos, sin pasar por la API).
func (c *Client) PresignedPutURL(ctx context.Context, objectKey string, expiry time.Duration) (string, error) {
	u, err := c.firmante.PresignedPutObject(ctx, c.bucket, objectKey, expiry)
	if err != nil {
		return "", fmt.Errorf("storage: no se pudo firmar PUT: %w", err)
	}
	return u.String(), nil
}

// PresignedGetURL emite una URL prefirmada de descarga temporal para
// materiales privados, después de verificar el derecho de acceso en la capa
// de aplicación.
func (c *Client) PresignedGetURL(ctx context.Context, objectKey string, expiry time.Duration, filename string) (string, error) {
	if c.publicURL != "" {
		return fmt.Sprintf("%s/%s", c.publicURL, objectKey), nil
	}
	reqParams := url.Values{}
	if filename != "" {
		reqParams.Set("response-content-disposition", fmt.Sprintf("inline; filename=%q", filename))
	}
	u, err := c.firmante.PresignedGetObject(ctx, c.bucket, objectKey, expiry, reqParams)
	if err != nil {
		return "", fmt.Errorf("storage: no se pudo firmar GET: %w", err)
	}
	return u.String(), nil
}

// InitiateMultipartUpload inicia una carga multipart reanudable.
func (c *Client) InitiateMultipartUpload(ctx context.Context, objectKey, contentType string) (uploadID string, err error) {
	core := minio.Core{Client: c.mc}
	uploadID, err = core.NewMultipartUpload(ctx, c.bucket, objectKey, minio.PutObjectOptions{ContentType: contentType})
	if err != nil {
		return "", fmt.Errorf("storage: no se pudo iniciar multipart: %w", err)
	}
	return uploadID, nil
}

// PresignedUploadPartURL firma una parte individual de una carga multipart,
// permitiendo que el cliente reanude la subida directamente hasta 24 horas.
func (c *Client) PresignedUploadPartURL(ctx context.Context, objectKey, uploadID string, partNumber int, expiry time.Duration) (string, error) {
	reqParams := url.Values{}
	reqParams.Set("partNumber", fmt.Sprintf("%d", partNumber))
	reqParams.Set("uploadId", uploadID)
	u, err := c.firmante.Presign(ctx, http.MethodPut, c.bucket, objectKey, expiry, reqParams)
	if err != nil {
		return "", fmt.Errorf("storage: no se pudo firmar la parte %d: %w", partNumber, err)
	}
	return u.String(), nil
}

// CompleteMultipartUpload finaliza la carga una vez el cliente confirma
// todas las partes con su ETag.
func (c *Client) CompleteMultipartUpload(ctx context.Context, objectKey, uploadID string, parts []minio.CompletePart) error {
	core := minio.Core{Client: c.mc}
	_, err := core.CompleteMultipartUpload(ctx, c.bucket, objectKey, uploadID, parts, minio.PutObjectOptions{})
	if err != nil {
		return fmt.Errorf("storage: no se pudo completar multipart: %w", err)
	}
	return nil
}

// DownloadToFile descarga un objeto completo a una ruta local; usado por los
// workers para operar sobre el original antes de transcodificar.
func (c *Client) DownloadToFile(ctx context.Context, objectKey, destPath string) error {
	if err := c.mc.FGetObject(ctx, c.bucket, objectKey, destPath, minio.GetObjectOptions{}); err != nil {
		return fmt.Errorf("storage: no se pudo descargar %s: %w", objectKey, err)
	}
	return nil
}

// UploadFile sube un archivo local (p.ej. un segmento HLS) a la clave dada.
func (c *Client) UploadFile(ctx context.Context, objectKey, srcPath, contentType string) error {
	_, err := c.mc.FPutObject(ctx, c.bucket, objectKey, srcPath, minio.PutObjectOptions{ContentType: contentType})
	if err != nil {
		return fmt.Errorf("storage: no se pudo subir %s: %w", objectKey, err)
	}
	return nil
}

// StatObject obtiene metadatos (tamaño, content-type) de un objeto ya
// cargado, usado para verificación de integridad tras la subida directa.
func (c *Client) StatObject(ctx context.Context, objectKey string) (minio.ObjectInfo, error) {
	return c.mc.StatObject(ctx, c.bucket, objectKey, minio.StatObjectOptions{})
}

// RemoveObject elimina un objeto (p.ej. tras fallo de escaneo antimalware).
func (c *Client) RemoveObject(ctx context.Context, objectKey string) error {
	return c.mc.RemoveObject(ctx, c.bucket, objectKey, minio.RemoveObjectOptions{})
}

// DetectMIME lee los primeros 512 bytes del objeto almacenado y devuelve el tipo
// MIME real detectado mediante números mágicos (magic bytes).
func (c *Client) DetectMIME(ctx context.Context, objectKey string) (string, error) {
	obj, err := c.mc.GetObject(ctx, c.bucket, objectKey, minio.GetObjectOptions{})
	if err != nil {
		return "", fmt.Errorf("storage: no se pudo abrir el objeto: %w", err)
	}
	defer obj.Close()

	buf := make([]byte, 512)
	n, err := io.ReadFull(obj, buf)
	if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
		return "", fmt.Errorf("storage: no se pudo leer cabecera: %w", err)
	}
	return http.DetectContentType(buf[:n]), nil
}

// ListObjectParts consulta las partes ya subidas a S3 de una carga multipart
// en curso, permitiendo que un cliente interrumpido reanude la subida.
func (c *Client) ListObjectParts(ctx context.Context, objectKey, uploadID string) (minio.ListObjectPartsResult, error) {
	core := minio.Core{Client: c.mc}
	return core.ListObjectParts(ctx, c.bucket, objectKey, uploadID, 0, 1000)
}

// AbrirObjeto devuelve el contenido del objeto para transmitirlo. Lo usa el
// escaneo antimalware, que necesita ver los bytes sin que la API los guarde en
// disco: el material de un curso puede pesar cientos de megas.
func (c *Client) AbrirObjeto(ctx context.Context, objectKey string) (io.ReadCloser, error) {
	obj, err := c.mc.GetObject(ctx, c.bucket, objectKey, minio.GetObjectOptions{})
	if err != nil {
		return nil, fmt.Errorf("storage: no se pudo abrir %s: %w", objectKey, err)
	}
	return obj, nil
}

// CalculateSHA256 calcula el checksum criptográfico SHA-256 de un objeto
// ensamblado en S3 para verificación rigurosa de integridad.
func (c *Client) CalculateSHA256(ctx context.Context, objectKey string) (string, error) {
	obj, err := c.mc.GetObject(ctx, c.bucket, objectKey, minio.GetObjectOptions{})
	if err != nil {
		return "", fmt.Errorf("storage: no se pudo abrir el objeto para hash: %w", err)
	}
	defer obj.Close()

	hasher := sha256.New()
	if _, err := io.Copy(hasher, obj); err != nil {
		return "", fmt.Errorf("storage: error calculando sha256: %w", err)
	}
	return hex.EncodeToString(hasher.Sum(nil)), nil
}

// Los tipos siguientes son el vocabulario propio del almacén. Existen para que
// las capas de arriba (handlers, casos de uso) hablen de objetos y de partes
// sin importar el SDK de un proveedor concreto, que es lo que pide la
// restricción de mantener el dominio desacoplado del proveedor cloud. La
// traducción desde y hacia los tipos de minio-go vive aquí y en ningún otro
// sitio.

// ObjetoInfo son los metadatos de un objeto ya cargado.
type ObjetoInfo struct {
	Tamano      int64
	ContentType string
	ETag        string
}

// ParteCargada identifica una parte confirmada por el cliente al cerrar una
// carga multipart.
type ParteCargada struct {
	Numero int
	ETag   string
}

// ParteEnCurso es una parte ya presente en el almacén, usada para que un
// cliente interrumpido sepa qué le falta por subir.
type ParteEnCurso struct {
	Numero int
	Tamano int64
	ETag   string
	Subida time.Time
}

// Metadatos devuelve los metadatos de un objeto en el vocabulario propio.
func (c *Client) Metadatos(ctx context.Context, objectKey string) (ObjetoInfo, error) {
	info, err := c.StatObject(ctx, objectKey)
	if err != nil {
		return ObjetoInfo{}, err
	}
	return ObjetoInfo{Tamano: info.Size, ContentType: info.ContentType, ETag: info.ETag}, nil
}

// CerrarCargaMultiparte finaliza la carga a partir de las partes confirmadas
// por el cliente.
func (c *Client) CerrarCargaMultiparte(ctx context.Context, objectKey, uploadID string, partes []ParteCargada) error {
	convertidas := make([]minio.CompletePart, len(partes))
	for i, p := range partes {
		convertidas[i] = minio.CompletePart{PartNumber: p.Numero, ETag: p.ETag}
	}
	return c.CompleteMultipartUpload(ctx, objectKey, uploadID, convertidas)
}

// PartesYaSubidas lista lo que el almacén ya tiene de una carga en curso.
func (c *Client) PartesYaSubidas(ctx context.Context, objectKey, uploadID string) ([]ParteEnCurso, error) {
	res, err := c.ListObjectParts(ctx, objectKey, uploadID)
	if err != nil {
		return nil, err
	}
	partes := make([]ParteEnCurso, 0, len(res.ObjectParts))
	for _, p := range res.ObjectParts {
		partes = append(partes, ParteEnCurso{
			Numero: p.PartNumber, Tamano: p.Size, ETag: p.ETag, Subida: p.LastModified,
		})
	}
	return partes, nil
}
