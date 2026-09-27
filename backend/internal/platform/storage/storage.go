// Package storage adapta el almacenamiento de objetos (S3/MinIO) para
// originales multimedia, derivados HLS, PDFs e imágenes de insignias.
// Ningún binario se persiste en PostgreSQL: la API solo emite URLs
// prefirmadas de subida/descarga.
package storage

import (
	"context"
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
	// SessionToken acompaña a AccessKey/SecretKey cuando son credenciales
	// temporales de STS (las de AWS Academy lo son): sin él, S3 rechaza la
	// firma con InvalidAccessKeyId aunque la llave y el secreto sean buenos.
	SessionToken string
	UseSSL       bool
	Bucket       string
	Region       string
	PublicURL    string
	// CrearBucket permite crear el bucket si falta. Ver asegurarBucket.
	CrearBucket bool

	// PublicEndpoint es el host por el que el navegador alcanza el almacén.
	// Vacío significa "el mismo que Endpoint".
	PublicEndpoint string
	PublicUseSSL   bool
}

func New(ctx context.Context, cfg Config) (*Client, error) {
	creds := credencialesDe(cfg)
	// Sin llave explícita, la cadena puede quedarse sin proveedor (sin
	// variables AWS_* y sin perfil de instancia) y minio-go seguiría en modo
	// anónimo sin avisar: el primer síntoma sería un AccessDenied sobre el
	// bucket, que apunta a la política y no a la falta de credenciales.
	if v, err := creds.Get(); err != nil || v.SignerType.IsAnonymous() {
		return nil, fmt.Errorf("storage: sin credenciales: S3_ACCESS_KEY vacío, sin AWS_ACCESS_KEY_ID y sin perfil de instancia (err: %v)", err)
	}

	mc, err := minio.New(cfg.Endpoint, &minio.Options{
		Creds:  creds,
		Secure: cfg.UseSSL,
		// Region explícita también aquí: sin ella minio-go averigua la región
		// del bucket con GetBucketLocation antes de cada operación nueva, lo
		// que exige un permiso IAM más y una petición extra que no aportan.
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

	return &Client{mc: mc, firmante: firmante, bucket: cfg.Bucket, publicURL: cfg.PublicURL}, nil
}

// asegurarBucket comprueba que el bucket existe y, solo si se permite, lo crea.
//
// En local crearlo es cómodo: MinIO arranca vacío. En AWS lo crea
// CloudFormation con su cifrado, su CORS y su bloqueo de acceso público, y un
// bucket creado aquí por un nombre mal escrito nacería sin nada de eso; por
// eso allí S3_CREATE_BUCKET=false convierte la ausencia en un error de
// arranque en lugar de en un bucket nuevo.
//
// Un AccessDenied al comprobarlo no tumba el proceso: HeadBucket exige
// s3:ListBucket, que una identidad limitada a objetos puede no tener sin que
// eso le impida trabajar. Si de verdad no puede, lo dirá la primera
// operación sobre un objeto, con el nombre de la clave.
func asegurarBucket(ctx context.Context, mc *minio.Client, cfg Config) error {
	exists, err := mc.BucketExists(ctx, cfg.Bucket)
	if err != nil {
		if minio.ToErrorResponse(err).Code == "AccessDenied" {
			return nil
		}
		return fmt.Errorf("storage: no se pudo verificar el bucket: %w", err)
	}
	if exists {
		return nil
	}
	if !cfg.CrearBucket {
		return fmt.Errorf("storage: el bucket %q no existe y S3_CREATE_BUCKET=false", cfg.Bucket)
	}
	if err := mc.MakeBucket(ctx, cfg.Bucket, minio.MakeBucketOptions{Region: regionDe(cfg)}); err != nil {
		return fmt.Errorf("storage: no se pudo crear el bucket: %w", err)
	}
	return nil
}

// credencialesDe elige de dónde salen las credenciales del almacén.
//
// Con S3_ACCESS_KEY definida se usan tal cual (MinIO en local, o una llave de
// IAM). Vacía, se delega en la cadena estándar de AWS: primero las variables
// AWS_* (credenciales temporales pegadas desde AWS Academy) y después el
// perfil de instancia de EC2. Lo segundo es lo que se busca en la nube: la
// llave rota sola, no vive en ningún .env y los permisos los fija el rol de
// la máquina, no quien escribió el archivo de configuración.
func credencialesDe(cfg Config) *credentials.Credentials {
	if cfg.AccessKey != "" {
		return credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, cfg.SessionToken)
	}
	return credentials.NewChainCredentials([]credentials.Provider{
		&credentials.EnvAWS{},
		&credentials.IAM{Client: &http.Client{Transport: http.DefaultTransport}},
	})
}

func regionDe(cfg Config) string {
	if cfg.Region == "" {
		return "us-east-1"
	}
	return cfg.Region
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
	//
	// Las credenciales son las mismas que las del cliente interno. Con el
	// perfil de instancia son temporales, y una URL firmada con ellas deja de
	// valer cuando caduca la sesión que la firmó, aunque su X-Amz-Expires
	// diga más: es un límite de S3, no de este código.
	c, err := minio.New(cfg.PublicEndpoint, &minio.Options{
		Creds:  credencialesDe(cfg),
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
func (c *Client) CompleteMultipartUpload(ctx context.Context, objectKey, uploadID string, partes []ParteCargada) error {
	core := minio.Core{Client: c.mc}
	completas := make([]minio.CompletePart, len(partes))
	for i, p := range partes {
		completas[i] = minio.CompletePart{PartNumber: p.Numero, ETag: p.ETag}
	}
	_, err := core.CompleteMultipartUpload(ctx, c.bucket, objectKey, uploadID, completas, minio.PutObjectOptions{})
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
	return core.AbortMultipartUpload(ctx, c.bucket, objectKey, uploadID)
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

// StatObject obtiene metadatos de un objeto ya cargado, usado para
// verificación de integridad tras la subida directa.
func (c *Client) StatObject(ctx context.Context, objectKey string) (ObjetoInfo, error) {
	info, err := c.mc.StatObject(ctx, c.bucket, objectKey, minio.StatObjectOptions{})
	if err != nil {
		return ObjetoInfo{}, err
	}
	return ObjetoInfo{Tamano: info.Size, ContentType: info.ContentType}, nil
}

// RemoveObject elimina un objeto (p.ej. tras fallo de escaneo antimalware).
func (c *Client) RemoveObject(ctx context.Context, objectKey string) error {
	return c.mc.RemoveObject(ctx, c.bucket, objectKey, minio.RemoveObjectOptions{})
}

// AbrirObjeto devuelve el contenido completo del objeto.
//
// Existe para que la verificación posterior a la carga —checksum, MIME real y
// escaneo antimalware— recorra el objeto una sola vez. Antes cada una de esas
// comprobaciones lo descargaba por su cuenta.
func (c *Client) AbrirObjeto(ctx context.Context, objectKey string) (io.ReadCloser, error) {
	obj, err := c.mc.GetObject(ctx, c.bucket, objectKey, minio.GetObjectOptions{})
	if err != nil {
		return nil, fmt.Errorf("storage: no se pudo abrir el objeto: %w", err)
	}
	return obj, nil
}

// GuardarObjeto escribe un objeto generado por la propia plataforma (hoy, la
// imagen de una insignia). Los binarios subidos por usuarios no pasan por
// aquí: esos van directos al almacén con una URL prefirmada.
func (c *Client) GuardarObjeto(ctx context.Context, objectKey string, r io.Reader, tamano int64, contentType string) error {
	_, err := c.mc.PutObject(ctx, c.bucket, objectKey, r, tamano, minio.PutObjectOptions{ContentType: contentType})
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
	res, err := core.ListObjectParts(ctx, c.bucket, objectKey, uploadID, 0, 1000)
	if err != nil {
		return nil, fmt.Errorf("storage: no se pudieron listar las partes: %w", err)
	}
	partes := make([]ParteCargada, 0, len(res.ObjectParts))
	for _, p := range res.ObjectParts {
		partes = append(partes, ParteCargada{Numero: p.PartNumber, ETag: p.ETag, Tamano: p.Size})
	}
	return partes, nil
}
