package config

import (
	"os"
	"testing"
)

// Ausente conserva la llave de MinIO para desarrollo local; definida y vacía
// llega vacía, y el almacén se niega a arrancar. Si ambas cosas se
// confundieran, un .env de GCP sin la clave HMAC firmaría con "minioadmin"
// contra GCS y todo acabaría en 403 sin decir por qué.
func TestLlaveS3AusenteFrenteAVacia(t *testing.T) {
	t.Run("ausente", func(t *testing.T) {
		// t.Setenv registra la restauración; después se borra de verdad.
		t.Setenv("S3_ACCESS_KEY", "x")
		t.Setenv("S3_SECRET_KEY", "x")
		unsetenv(t, "S3_ACCESS_KEY")
		unsetenv(t, "S3_SECRET_KEY")
		if c := Load(); c.S3AccessKey != "minioadmin" || c.S3SecretKey != "minioadmin" {
			t.Fatalf("esperaba la llave local por defecto, obtuve %q/%q", c.S3AccessKey, c.S3SecretKey)
		}
	})
	t.Run("vacía", func(t *testing.T) {
		t.Setenv("S3_ACCESS_KEY", "")
		t.Setenv("S3_SECRET_KEY", "")
		if c := Load(); c.S3AccessKey != "" || c.S3SecretKey != "" {
			t.Fatalf("esperaba llave vacía, obtuve %q/%q", c.S3AccessKey, c.S3SecretKey)
		}
	})
}

func unsetenv(t *testing.T, key string) {
	t.Helper()
	if err := os.Unsetenv(key); err != nil {
		t.Fatalf("Unsetenv(%s): %v", key, err)
	}
}

// En las VM de 2 GB la concurrencia baja a 1-2 por memoria; sin variable se conserva el 5
// de siempre para no cambiar el comportamiento local.
func TestConcurrenciaDelWorker(t *testing.T) {
	t.Setenv("WORKER_CONCURRENCY", "")
	if c := Load(); c.WorkerConcurrency != 5 {
		t.Fatalf("por defecto: %d, esperaba 5", c.WorkerConcurrency)
	}
	t.Setenv("WORKER_CONCURRENCY", "2")
	if c := Load(); c.WorkerConcurrency != 2 {
		t.Fatalf("con WORKER_CONCURRENCY=2: %d", c.WorkerConcurrency)
	}
}

// El pool por proceso se suma entre API y worker contra el max_connections de
// Cloud SQL; sin variable se conserva el 20 de siempre.
func TestConexionesDelPool(t *testing.T) {
	t.Setenv("DB_MAX_CONNS", "")
	if c := Load(); c.DBMaxConns != 20 {
		t.Fatalf("por defecto: %d, esperaba 20", c.DBMaxConns)
	}
	t.Setenv("DB_MAX_CONNS", "15")
	if c := Load(); c.DBMaxConns != 15 {
		t.Fatalf("con DB_MAX_CONNS=15: %d", c.DBMaxConns)
	}
}

// Vacío deja un solo bucket (local); con valor, hls/* va aparte (GCP).
func TestBucketHLS(t *testing.T) {
	t.Setenv("S3_BUCKET_HLS", "")
	if c := Load(); c.S3BucketHLS != "" {
		t.Fatalf("por defecto: %q, esperaba vacío", c.S3BucketHLS)
	}
	t.Setenv("S3_BUCKET_HLS", "p-mooc-hls")
	if c := Load(); c.S3BucketHLS != "p-mooc-hls" {
		t.Fatalf("S3_BUCKET_HLS no llegó: %q", c.S3BucketHLS)
	}
}
