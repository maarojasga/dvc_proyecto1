// Package storage adapta el almacenamiento de objetos (S3/MinIO) para
// originales multimedia, derivados HLS, PDFs e imágenes de insignias.
// Ningún binario se persiste en PostgreSQL: la API solo emite URLs
// prefirmadas de subida/descarga.
package storage

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

type Client struct {
	mc        *minio.Client
	bucket    string
	publicURL string // si está vacío, se usan URLs prefirmadas también para GET
}

type Config struct {
	Endpoint  string
	AccessKey string
	SecretKey string
	UseSSL    bool
	Bucket    string
	PublicURL string
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

	return &Client{mc: mc, bucket: cfg.Bucket, publicURL: cfg.PublicURL}, nil
}

// SirveDesdeCDN informa si los objetos se entregan por una base pública en
// lugar de firmarse uno a uno. Lo expone la API para que el cliente sepa si
// la URL que recibe caduca.
func (c *Client) SirveDesdeCDN() bool { return c.publicURL != "" }

// PresignedPutURL emite una URL prefirmada de subida directa (carga
// multipart directa a objetos, sin pasar por la API).
func (c *Client) PresignedPutURL(ctx context.Context, objectKey string, expiry time.Duration) (string, error) {
	u, err := c.mc.PresignedPutObject(ctx, c.bucket, objectKey, expiry)
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
	u, err := c.mc.PresignedGetObject(ctx, c.bucket, objectKey, expiry, reqParams)
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
	u, err := c.mc.Presign(ctx, http.MethodPut, c.bucket, objectKey, expiry, reqParams)
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
