package config

import (
	"os"
	"testing"
)

// Ausente conserva la llave de MinIO para desarrollo local; definida y vacía
// pide la cadena de credenciales de AWS. Si ambas cosas se confundieran, en
// EC2 la API firmaría con "minioadmin" contra S3 y todo acabaría en 403.
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
	t.Run("token de sesión", func(t *testing.T) {
		t.Setenv("S3_SESSION_TOKEN", "token")
		if c := Load(); c.S3SessionToken != "token" {
			t.Fatalf("S3_SESSION_TOKEN no llegó a la configuración: %q", c.S3SessionToken)
		}
	})
}

func unsetenv(t *testing.T, key string) {
	t.Helper()
	if err := os.Unsetenv(key); err != nil {
		t.Fatalf("Unsetenv(%s): %v", key, err)
	}
}

// En AWS la concurrencia baja a 2 por memoria; sin variable se conserva el 5
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
