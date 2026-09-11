// Package config centraliza la lectura de variables de entorno para la API y los workers.
package config

import (
	"os"
	"strconv"
	"time"
)

// Config agrupa los parámetros de configuración compartidos por la API y los workers.
type Config struct {
	Env         string
	HTTPPort    string
	DatabaseURL string
	RedisAddr   string

	S3Endpoint  string
	S3Bucket    string
	S3AccessKey string
	S3SecretKey string
	S3UseSSL    bool
	S3Region    string
	S3PublicURL string // base URL pública/CDN para servir objetos (opcional)

	// S3PublicEndpoint es el host por el que el NAVEGADOR alcanza el
	// almacén de objetos. Dentro de Docker, S3Endpoint es "minio:9000",
	// un nombre que sólo resuelve en la red del compose; una URL
	// prefirmada contra ese host llega al navegador y muere en
	// ERR_NAME_NOT_RESOLVED. La firma SigV4 cubre el Host, así que no
	// basta con reescribir la URL después: hay que firmar contra el host
	// público desde el principio.
	S3PublicEndpoint string
	S3PublicUseSSL   bool

	// AntimalwareAddr es la dirección del demonio ClamAV (host:puerto). Si
	// queda vacía se usa el escáner de desarrollo, y el arranque en
	// producción lo rechaza: un escáner que aprueba todo da la apariencia del
	// control sin el control.
	AntimalwareAddr string

	SMTPHost string
	SMTPPort string
	SMTPFrom string

	SessionTTL    time.Duration
	PublicBaseURL string // URL pública del frontend, para links de verificación/reseteo
	CookieSecure  bool
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
		S3AccessKey: getEnv("S3_ACCESS_KEY", "minioadmin"),
		S3SecretKey: getEnv("S3_SECRET_KEY", "minioadmin"),
		S3UseSSL:    getBool("S3_USE_SSL", false),
		S3Region:    getEnv("S3_REGION", "us-east-1"),
		S3PublicURL: getEnv("S3_PUBLIC_URL", ""),

		S3PublicEndpoint: getEnv("S3_PUBLIC_ENDPOINT", ""),
		S3PublicUseSSL:   getBool("S3_PUBLIC_USE_SSL", getBool("S3_USE_SSL", false)),

		AntimalwareAddr: getEnv("ANTIMALWARE_ADDR", ""),

		SMTPHost: getEnv("SMTP_HOST", "localhost"),
		SMTPPort: getEnv("SMTP_PORT", "1025"),
		SMTPFrom: getEnv("SMTP_FROM", "no-reply@mooc.local"),

		SessionTTL:    getDuration("SESSION_TTL", 30*24*time.Hour),
		PublicBaseURL: getEnv("PUBLIC_BASE_URL", "http://localhost:3000"),
		CookieSecure:  getBool("COOKIE_SECURE", false),
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getBool(key string, fallback bool) bool {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return fallback
	}
	return b
}

func getDuration(key string, fallback time.Duration) time.Duration {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return fallback
	}
	return d
}
