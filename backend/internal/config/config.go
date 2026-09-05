// Package config centraliza la lectura de variables de entorno para la API y los workers.
package config

import (
	"os"
	"strconv"
	"strings"
)

// Config agrupa los parametros de configuracion compartidos por la API y los workers.
//
// Todo se lee del entorno: ningun secreto vive en el repositorio ni en la
// imagen, que es lo que exige la gestion externa de secretos.
type Config struct {
	Env         string
	HTTPPort    string
	DatabaseURL string
	RedisAddr   string
	S3Endpoint  string
	S3Bucket    string

	// URLFrontend es el origen publico del frontend. Se usa para construir
	// los enlaces de los correos transaccionales.
	URLFrontend string

	SMTPDireccion string
	SMTPRemitente string
	SMTPUsuario   string
	SMTPClave     string

	// CookieSegura marca las cookies de sesion como Secure. Se desactiva en
	// desarrollo, donde no hay TLS.
	CookieSegura bool
}

// Load construye la configuracion a partir de variables de entorno, con valores
// por defecto razonables para desarrollo local.
func Load() Config {
	env := getEnv("APP_ENV", "development")
	return Config{
		Env:         env,
		HTTPPort:    getEnv("API_PORT", "8080"),
		DatabaseURL: getEnv("DATABASE_URL", "postgres://mooc:mooc@localhost:5432/mooc?sslmode=disable"),
		RedisAddr:   getEnv("REDIS_ADDR", "localhost:6379"),
		S3Endpoint:  getEnv("S3_ENDPOINT", "localhost:9000"),
		S3Bucket:    getEnv("S3_BUCKET", "mooc"),

		URLFrontend: getEnv("FRONTEND_URL", "http://localhost:3000"),

		// SMTP_ADDR admite la cadena vacia como valor deliberado: significa
		// "sin servidor de correo", y hace que la API registre los enlaces en
		// el log en lugar de enviarlos.
		SMTPDireccion: getEnvOpcional("SMTP_ADDR", "localhost:1025"),
		SMTPRemitente: getEnv("SMTP_FROM", "no-responder@mooc.local"),
		SMTPUsuario:   getEnv("SMTP_USER", ""),
		SMTPClave:     getEnv("SMTP_PASSWORD", ""),

		CookieSegura: getBool("COOKIE_SECURE", env != "development"),
	}
}

// EsDesarrollo informa si la instancia corre en el entorno local.
func (c Config) EsDesarrollo() bool { return c.Env == "development" }

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// getEnvOpcional distingue una variable ausente de una fijada a la cadena
// vacia: si esta declarada, se respeta su valor aunque sea vacio.
func getEnvOpcional(key, fallback string) string {
	if v, hay := os.LookupEnv(key); hay {
		return v
	}
	return fallback
}

func getBool(key string, fallback bool) bool {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return fallback
	}
	return b
}
