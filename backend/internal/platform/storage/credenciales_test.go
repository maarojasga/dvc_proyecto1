package storage

import (
	"context"
	"net/url"
	"strings"
	"testing"
	"time"
)

// La llave es siempre explícita: sin ella New tiene que fallar al arrancar.
// Si cayera en modo anónimo, minio-go haría las peticiones sin firmar y el
// primer síntoma sería un AccessDenied sobre un objeto, que apunta a los
// permisos del bucket y no a la configuración.
func TestSinLlaveNoSeArranca(t *testing.T) {
	for _, cfg := range []Config{
		{},
		{AccessKey: "GOOG1E-PRUEBA"},
		{SecretKey: "secreto"},
	} {
		if _, err := credencialesDe(cfg); err == nil {
			t.Errorf("credencialesDe(%+v) no falló", cfg)
		}
		if _, err := New(context.Background(), cfg); err == nil {
			t.Errorf("New(%+v) arrancó sin credenciales", cfg)
		}
	}
}

func TestLlaveExplicita(t *testing.T) {
	creds, err := credencialesDe(Config{AccessKey: "GOOG1E-PRUEBA", SecretKey: "secreto"})
	if err != nil {
		t.Fatalf("credencialesDe: %v", err)
	}
	v, err := creds.Get()
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if v.AccessKeyID != "GOOG1E-PRUEBA" || v.SignerType.IsAnonymous() {
		t.Fatalf("credenciales inesperadas: %+v", v)
	}
}

// GCS: el endpoint tiene que ser exactamente storage.googleapis.com (minio-go
// solo lo reconoce así) y la región del ámbito SigV4, "auto". La URL sale en
// estilo virtual-host, <bucket>.storage.googleapis.com, que es la forma que
// el CORS del bucket y el certificado comodín de Google esperan.
func TestFirmaContraGCS(t *testing.T) {
	cfg := Config{
		Endpoint: "storage.googleapis.com", UseSSL: true,
		AccessKey: "GOOG1E-PRUEBA", SecretKey: "secreto",
		Bucket: "proyecto-mooc-objetos", BucketHLS: "proyecto-mooc-hls", Region: "auto",
	}
	f, err := clienteDeFirma(Config{
		Endpoint: "interno.invalid", PublicEndpoint: cfg.Endpoint, PublicUseSSL: true,
		AccessKey: cfg.AccessKey, SecretKey: cfg.SecretKey, Region: cfg.Region,
	}, nil)
	if err != nil {
		t.Fatalf("clienteDeFirma: %v", err)
	}
	c := &Client{firmante: f, bucket: cfg.Bucket, bucketHLS: cfg.BucketHLS}
	ctx := context.Background()

	get, err := c.PresignedGetURL(ctx, "presentaciones/abc/preview.pdf", time.Hour, "clase.pdf")
	if err != nil {
		t.Fatalf("PresignedGetURL: %v", err)
	}
	u, _ := url.Parse(get)
	if u.Scheme != "https" || u.Host != "proyecto-mooc-objetos.storage.googleapis.com" {
		t.Errorf("GET firmado contra %s://%s", u.Scheme, u.Host)
	}
	q := u.Query()
	if !strings.Contains(q.Get("X-Amz-Credential"), "/auto/s3/aws4_request") {
		t.Errorf("ámbito de la credencial sin región auto: %s", q.Get("X-Amz-Credential"))
	}
	// GCS admite response-content-disposition en la API XML; tiene que ir
	// dentro de lo firmado, no añadido después.
	if !strings.Contains(q.Get("response-content-disposition"), "clase.pdf") {
		t.Errorf("falta response-content-disposition: %s", get)
	}

	master, err := c.PresignedGetURL(ctx, "hls/abc/master.m3u8", time.Hour, "")
	if err != nil {
		t.Fatalf("PresignedGetURL(hls): %v", err)
	}
	if h := hostDe(t, master); h != "proyecto-mooc-hls.storage.googleapis.com" {
		t.Errorf("la lista maestra HLS se firmó contra %s, esperaba el bucket HLS", h)
	}
}

// Las claves no cambian con el bucket: hls/* va al bucket HLS si lo hay, y
// todo lo demás (originales, PDFs, subtítulos, insignias) al privado. Un
// original en el bucket público sería legible sin autorización.
func TestBucketPorClave(t *testing.T) {
	c := &Client{bucket: "privado", bucketHLS: "publico-hls"}
	casos := map[string]string{
		"hls/abc/master.m3u8":            "publico-hls",
		"hls/abc/720p_000.ts":            "publico-hls",
		"resources/abc/original":         "privado",
		"presentaciones/abc/preview.pdf": "privado",
		"subtitulos/abc/es.vtt":          "privado",
		"badges/XYZ.svg":                 "privado",
		// Solo el prefijo exacto: una clave que contiene hls/ más adelante
		// no es un derivado.
		"resources/hls/original": "privado",
	}
	for clave, quiero := range casos {
		if got := c.bucketDe(clave); got != quiero {
			t.Errorf("bucketDe(%q) = %q, esperaba %q", clave, got, quiero)
		}
	}

	// Sin bucket HLS (local, MinIO) todo va al único bucket.
	solo := &Client{bucket: "mooc"}
	if got := solo.bucketDe("hls/abc/master.m3u8"); got != "mooc" {
		t.Errorf("sin bucket HLS: %q", got)
	}
}
