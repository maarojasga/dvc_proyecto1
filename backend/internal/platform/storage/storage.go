// Package storage adapta el almacenamiento de objetos (MinIO en local, Cloud
// Storage por su API XML compatible con S3 en GCP) para originales
// multimedia, derivados HLS, PDFs e imágenes de insignias.
// Ningún binario se persiste en PostgreSQL: la API solo emite URLs
// prefirmadas de subida/descarga.
package storage

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

type Client struct {
	mc     *minio.Client
	bucket string
	// bucketHLS recibe las claves hls/*. Vacío significa "el mismo bucket".
	// Ver bucketDe.
	bucketHLS string

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
	// BucketHLS, si no está vacío, es el bucket donde viven los derivados
	// HLS (claves hls/*). Existe por GCS: con acceso uniforme a nivel de
	// bucket no se puede abrir a lectura anónima un solo prefijo, y el
	// reproductor pide variantes y segmentos sin firma. Ver bucketDe.
	BucketHLS string
	Region    string
	PublicURL string
	// CrearBucket permite crear el bucket si falta. Ver asegurarBucket.
	CrearBucket bool

	// PublicEndpoint es el host por el que el navegador alcanza el almacén.
	// Vacío significa "el mismo que Endpoint".
	PublicEndpoint string
	PublicUseSSL   bool
}

func New(ctx context.Context, cfg Config) (*Client, error) {
	creds, err := credencialesDe(cfg)
	if err != nil {
		return nil, err
	}

	mc, err := minio.New(cfg.Endpoint, &minio.Options{
		Creds:  creds,
		Secure: cfg.UseSSL,
		// Region explícita también aquí: sin ella minio-go averigua la región
		// del bucket con GetBucketLocation antes de cada operación nueva, lo
		// que exige un permiso más (storage.buckets.get en GCS) y una
		// petición extra que no aportan.
		Region: regionDe(cfg),
	})
	if err != nil {
		return nil, fmt.Errorf("storage: no se pudo crear el cliente: %w", err)
	}

	if err := asegurarBucket(ctx, mc, cfg); err != nil {
		return nil, err
	}

	firmante, err := clienteDeFirma(cfg, mc)
	if err != nil {
		return nil, err
	}

	return &Client{mc: mc, firmante: firmante, bucket: cfg.Bucket, bucketHLS: cfg.BucketHLS, publicURL: cfg.PublicURL}, nil
}

// asegurarBucket comprueba que los buckets existen y, solo si se permite, los
// crea.
//
// En local crearlo es cómodo: MinIO arranca vacío. En GCP los crea Terraform
// con su CORS, su ciclo de vida y su prevención de acceso público, y un
// bucket creado aquí por un nombre mal escrito nacería sin nada de eso; por
// eso allí S3_CREATE_BUCKET=false convierte la ausencia en un error de
// arranque en lugar de en un bucket nuevo.
//
// Un AccessDenied al comprobarlo no tumba el proceso: HeadBucket exige
// storage.buckets.get, que no está en ningún rol storage.object* y que las
// cuentas de servicio de la API y el worker no tienen a propósito. Si de
// verdad no pueden trabajar, lo dirá la primera operación sobre un objeto,
// con el nombre de la clave.
func asegurarBucket(ctx context.Context, mc *minio.Client, cfg Config) error {
	buckets := []string{cfg.Bucket}
	if cfg.BucketHLS != "" && cfg.BucketHLS != cfg.Bucket {
		buckets = append(buckets, cfg.BucketHLS)
	}
	for _, b := range buckets {
		exists, err := mc.BucketExists(ctx, b)
		if err != nil {
			if minio.ToErrorResponse(err).Code == "AccessDenied" {
				continue
			}
			return fmt.Errorf("storage: no se pudo verificar el bucket %q: %w", b, err)
		}
		if exists {
			continue
		}
		if !cfg.CrearBucket {
			return fmt.Errorf("storage: el bucket %q no existe y S3_CREATE_BUCKET=false", b)
		}
		if err := mc.MakeBucket(ctx, b, minio.MakeBucketOptions{Region: regionDe(cfg)}); err != nil {
			return fmt.Errorf("storage: no se pudo crear el bucket %q: %w", b, err)
		}
	}
	return nil
}

// credencialesDe devuelve la llave estática con la que se firma todo.
//
// Es siempre explícita (S3_ACCESS_KEY/S3_SECRET_KEY): la llave de MinIO en
// local y una clave HMAC de cuenta de servicio en GCS. No hay cadena de
// proveedores a propósito: la de minio-go termina en el servicio de metadatos
// de EC2, que en Compute Engine no existe (169.254.169.254 responde, pero con
// otra API), y lo único que se conseguiría es un arranque lento que acaba en
// modo anónimo. Sin llave, minio-go seguiría sin firmar y sin avisar: el
// primer síntoma sería un AccessDenied sobre el bucket, que apunta a los
// permisos y no a la configuración. Por eso falla aquí.
func credencialesDe(cfg Config) (*credentials.Credentials, error) {
	if cfg.AccessKey == "" || cfg.SecretKey == "" {
		return nil, fmt.Errorf("storage: sin credenciales: S3_ACCESS_KEY y S3_SECRET_KEY son obligatorias (en GCP, la clave HMAC de la cuenta de servicio)")
	}
	return credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""), nil
}

// regionDe es la región con la que se firma. MinIO acepta cualquiera y usa
// us-east-1 por defecto; GCS espera "auto" en el ámbito de la credencial
// SigV4 (S3_REGION=auto en los .env de GCP).
func regionDe(cfg Config) string {
	if cfg.Region == "" {
		return "us-east-1"
	}
	return cfg.Region
}

// bucketDe decide en qué bucket vive una clave.
//
// Las claves no cambian (hls/<recurso>/master.m3u8 sigue siendo la que guarda
// la base), solo el bucket que las aloja: así la organización lógica de los
// objetos es la misma en local, donde todo va a un bucket con hls/ abierto,
// y en GCS, donde hls/ va a un bucket aparte legible por cualquiera y el
// resto queda en uno privado con acceso solo por URL firmada.
func (c *Client) bucketDe(clave string) string {
	if c.bucketHLS != "" && strings.HasPrefix(clave, prefijoHLS) {
		return c.bucketHLS
	}
	return c.bucket
}

// prefijoHLS es el de los derivados que escribe el worker de medios
// (media.Processor.uploadDir).
const prefijoHLS = "hls/"

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
	creds, err := credencialesDe(cfg)
	if err != nil {
		return nil, err
	}
	c, err := minio.New(cfg.PublicEndpoint, &minio.Options{
		Creds:  creds,
		Secure: cfg.PublicUseSSL,
		Region: regionDe(cfg),
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
	u, err := c.firmante.PresignedPutObject(ctx, c.bucketDe(objectKey), objectKey, expiry)
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
	u, err := c.firmante.PresignedGetObject(ctx, c.bucketDe(objectKey), objectKey, expiry, reqParams)
	if err != nil {
		return "", fmt.Errorf("storage: no se pudo firmar GET: %w", err)
	}
	return u.String(), nil
}

// InitiateMultipartUpload inicia una carga multipart reanudable.
func (c *Client) InitiateMultipartUpload(ctx context.Context, objectKey, contentType string) (uploadID string, err error) {
	core := minio.Core{Client: c.mc}
	uploadID, err = core.NewMultipartUpload(ctx, c.bucketDe(objectKey), objectKey, minio.PutObjectOptions{ContentType: contentType})
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
	u, err := c.firmante.Presign(ctx, http.MethodPut, c.bucketDe(objectKey), objectKey, expiry, reqParams)
	if err != nil {
		return "", fmt.Errorf("storage: no se pudo firmar la parte %d: %w", partNumber, err)
	}
	return u.String(), nil
}

// CompleteMultipartUpload finaliza la carga una vez el cliente confirma
// todas las partes con su ETag.
func (c *Client) CompleteMultipartUpload(ctx context.Context, objectKey, uploadID string, partes []ParteCargada) error {
	core := minio.Core{Client: c.mc}
	completas := make([]minio.CompletePart, len(partes))
	for i, p := range partes {
		completas[i] = minio.CompletePart{PartNumber: p.Numero, ETag: p.ETag}
	}
	_, err := core.CompleteMultipartUpload(ctx, c.bucketDe(objectKey), objectKey, uploadID, completas, minio.PutObjectOptions{})
	if err != nil {
		return fmt.Errorf("storage: no se pudo completar multipart: %w", err)
	}
	return nil
}

// AbortMultipartUpload descarta una carga multipart a medias y libera las
// partes ya subidas. Se llama cuando la verificación rechaza el objeto: sin
// esto, el almacén conserva y factura partes de un archivo que nunca existió.
func (c *Client) AbortMultipartUpload(ctx context.Context, objectKey, uploadID string) error {
	core := minio.Core{Client: c.mc}
	return core.AbortMultipartUpload(ctx, c.bucketDe(objectKey), objectKey, uploadID)
}

// DownloadToFile descarga un objeto completo a una ruta local; usado por los
// workers para operar sobre el original antes de transcodificar.
func (c *Client) DownloadToFile(ctx context.Context, objectKey, destPath string) error {
	if err := c.mc.FGetObject(ctx, c.bucketDe(objectKey), objectKey, destPath, minio.GetObjectOptions{}); err != nil {
		return fmt.Errorf("storage: no se pudo descargar %s: %w", objectKey, err)
	}
	return nil
}

// UploadFile sube un archivo local (p.ej. un segmento HLS) a la clave dada.
func (c *Client) UploadFile(ctx context.Context, objectKey, srcPath, contentType string) error {
	_, err := c.mc.FPutObject(ctx, c.bucketDe(objectKey), objectKey, srcPath, minio.PutObjectOptions{ContentType: contentType})
	if err != nil {
		return fmt.Errorf("storage: no se pudo subir %s: %w", objectKey, err)
	}
	return nil
}

// StatObject obtiene metadatos de un objeto ya cargado, usado para
// verificación de integridad tras la subida directa.
func (c *Client) StatObject(ctx context.Context, objectKey string) (ObjetoInfo, error) {
	info, err := c.mc.StatObject(ctx, c.bucketDe(objectKey), objectKey, minio.StatObjectOptions{})
	if err != nil {
		return ObjetoInfo{}, err
	}
	return ObjetoInfo{Tamano: info.Size, ContentType: info.ContentType}, nil
}

// RemoveObject elimina un objeto (p.ej. tras fallo de escaneo antimalware).
func (c *Client) RemoveObject(ctx context.Context, objectKey string) error {
	return c.mc.RemoveObject(ctx, c.bucketDe(objectKey), objectKey, minio.RemoveObjectOptions{})
}

// AbrirObjeto devuelve el contenido completo del objeto.
//
// Existe para que la verificación posterior a la carga —checksum, MIME real y
// escaneo antimalware— recorra el objeto una sola vez. Antes cada una de esas
// comprobaciones lo descargaba por su cuenta.
func (c *Client) AbrirObjeto(ctx context.Context, objectKey string) (io.ReadCloser, error) {
	obj, err := c.mc.GetObject(ctx, c.bucketDe(objectKey), objectKey, minio.GetObjectOptions{})
	if err != nil {
		return nil, fmt.Errorf("storage: no se pudo abrir el objeto: %w", err)
	}
	return obj, nil
}

// GuardarObjeto escribe un objeto generado por la propia plataforma (hoy, la
// imagen de una insignia). Los binarios subidos por usuarios no pasan por
// aquí: esos van directos al almacén con una URL prefirmada.
func (c *Client) GuardarObjeto(ctx context.Context, objectKey string, r io.Reader, tamano int64, contentType string) error {
	_, err := c.mc.PutObject(ctx, c.bucketDe(objectKey), objectKey, r, tamano, minio.PutObjectOptions{ContentType: contentType})
	if err != nil {
		return fmt.Errorf("storage: no se pudo guardar %s: %w", objectKey, err)
	}
	return nil
}

// ListObjectParts consulta las partes ya subidas de una carga multipart en
// curso, permitiendo que un cliente interrumpido reanude la subida sin
// reenviar lo que ya llegó.
func (c *Client) ListObjectParts(ctx context.Context, objectKey, uploadID string) ([]ParteCargada, error) {
	core := minio.Core{Client: c.mc}
	res, err := core.ListObjectParts(ctx, c.bucketDe(objectKey), objectKey, uploadID, 0, 1000)
	if err != nil {
		return nil, fmt.Errorf("storage: no se pudieron listar las partes: %w", err)
	}
	partes := make([]ParteCargada, 0, len(res.ObjectParts))
	for _, p := range res.ObjectParts {
		partes = append(partes, ParteCargada{Numero: p.PartNumber, ETag: p.ETag, Tamano: p.Size})
	}
	return partes, nil
}
