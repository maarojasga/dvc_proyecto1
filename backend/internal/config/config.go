// Package config centraliza la lectura de variables de entorno para la API y los workers.
package config

import "os"

// Config agrupa los parámetros de configuración compartidos por la API y los workers.
type Config struct {
	Env         string
	HTTPPort    string
	DatabaseURL string
	RedisAddr   string
	S3Endpoint  string
	S3Bucket    string
}

// Load construye la configuración a partir de variables de entorno, con valores
// por defecto razonables para desarrollo local.
func Load() Config {
	return Config{
		Env:         getEnv("APP_ENV", "development"),
		HTTPPort:    getEnv("API_PORT", "8080"),
		DatabaseURL: getEnv("DATABASE_URL", "postgres://mooc:mooc@localhost:5432/mooc?sslmode=disable"),
		RedisAddr:   getEnv("REDIS_ADDR", "localhost:6379"),
		S3Endpoint:  getEnv("S3_ENDPOINT", "localhost:9000"),
		S3Bucket:    getEnv("S3_BUCKET", "mooc"),
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
