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
	// DBMaxConns es el tamaño del pool de PostgreSQL de cada proceso. Se
	// suma entre la API y el worker y tiene que caber en el max_connections
	// de la base administrada, que depende del tamaño de la instancia (en
	// Cloud SQL, 25 en db-f1-micro y 50 en db-g1-small).
	DBMaxConns int
	RedisAddr  string

	S3Endpoint string
	S3Bucket   string
	// S3BucketHLS, si no está vacío, aloja las claves hls/*. En GCP es un
	// bucket aparte y legible por cualquiera, porque el reproductor pide los
	// segmentos sin firma; ver storage.Config.BucketHLS.
	S3BucketHLS string
	// S3AccessKey vacía (definida pero sin valor) es un error de arranque, no
	// un "usar otra cosa": en la nube tiene que llegar la clave HMAC y un
	// olvido no debe acabar firmando con la llave local de MinIO.
	S3AccessKey string
	S3SecretKey string
	S3UseSSL    bool
	// S3CreateBucket permite que la API y el worker creen el bucket si no
	// existe. Cierto en local (MinIO arranca vacío); falso en GCP, donde los
	// buckets los crea Terraform con su CORS, su ciclo de vida y su
	// prevención de acceso público.
	S3CreateBucket bool
	S3Region       string
	S3PublicURL    string // base URL pública/CDN para servir objetos (opcional)

	// S3PublicEndpoint es el host por el que el NAVEGADOR alcanza el
	// almacén de objetos. Dentro de Docker, S3Endpoint es "minio:9000",
	// un nombre que sólo resuelve en la red del compose; una URL
	// prefirmada contra ese host llega al navegador y muere en
	// ERR_NAME_NOT_RESOLVED. La firma SigV4 cubre el Host, así que no
	// basta con reescribir la URL después: hay que firmar contra el host
	// público desde el principio.
	S3PublicEndpoint string
	S3PublicUseSSL   bool

	// ClamAVAddr es la dirección de clamd (host:puerto). Vacía deja operando
	// el escáner antimalware integrado, que reconoce el vector de prueba
	// estándar y los formatos ejecutables pero no sustituye a un antivirus
	// con firmas actualizadas.
	ClamAVAddr string

	// BadgeSigningKey es la clave privada Ed25519 con que se firman las
	// credenciales Open Badges 3.0, en base64. Vacía desactiva la credencial
	// portátil; la insignia sigue siendo verificable por su URL pública.
	//
	// Es material criptográfico, así que viene de una variable de entorno y no
	// del repositorio: la gestión externa de secretos es una restricción
	// técnica del proyecto.
	BadgeSigningKey string
	// BadgeKeyID identifica la clave dentro del JWKS, para poder rotarla sin
	// invalidar de golpe lo firmado antes.
	BadgeKeyID string
	// IssuerName es la organización que figura como emisora.
	IssuerName string

	SMTPHost string
	SMTPPort string
	SMTPFrom string

	SessionTTL    time.Duration
	PublicBaseURL string // URL pública del frontend, para links de verificación/reseteo
	CookieSecure  bool
	// AuthRateLimitPerMinute acota los intentos de registro, login y
	// restablecimiento por IP y minuto. Es configurable porque el valor bueno
	// depende del despliegue: diez protege una instalación real, pero una
	// suite E2E que crea decenas de cuentas desde una sola IP se ahoga con él y
	// acabaría midiendo el limitador en vez del producto. El valor por defecto
	// es el de producción; subirlo es una decisión explícita del entorno.
	AuthRateLimitPerMinute int

	// WorkerConcurrency es el número de trabajos que un worker procesa a la
	// vez. Cada uno puede ser un FFmpeg o un LibreOffice, así que el valor
	// correcto depende de la memoria de la máquina: 5 cabe en un portátil,
	// pero en una VM de 2 GiB varios FFmpeg a la vez acaban en el OOM killer.
	WorkerConcurrency int
}

// Load construye la configuración a partir de variables de entorno, con valores
// por defecto razonables para desarrollo local.
func Load() Config {
	return Config{
		Env:         getEnv("APP_ENV", "development"),
		HTTPPort:    getEnv("API_PORT", "8080"),
		DatabaseURL: getEnv("DATABASE_URL", "postgres://mooc:mooc@localhost:5432/mooc?sslmode=disable"),
		DBMaxConns:  getInt("DB_MAX_CONNS", 20),
		RedisAddr:   getEnv("REDIS_ADDR", "localhost:6379"),

		S3Endpoint:  getEnv("S3_ENDPOINT", "localhost:9000"),
		S3Bucket:    getEnv("S3_BUCKET", "mooc"),
		S3BucketHLS: getEnv("S3_BUCKET_HLS", ""),
		// Definida y vacía no es lo mismo que ausente: ausente conserva la
		// llave de MinIO para `go run` en local; vacía llega vacía y el
		// almacén se niega a arrancar sin credenciales.
		S3AccessKey:    getEnvDefinida("S3_ACCESS_KEY", "minioadmin"),
		S3SecretKey:    getEnvDefinida("S3_SECRET_KEY", "minioadmin"),
		S3UseSSL:       getBool("S3_USE_SSL", false),
		S3CreateBucket: getBool("S3_CREATE_BUCKET", true),
		S3Region:       getEnv("S3_REGION", "us-east-1"),
		S3PublicURL:    getEnv("S3_PUBLIC_URL", ""),

		S3PublicEndpoint: getEnv("S3_PUBLIC_ENDPOINT", ""),
		S3PublicUseSSL:   getBool("S3_PUBLIC_USE_SSL", getBool("S3_USE_SSL", false)),

		ClamAVAddr: getEnv("CLAMAV_ADDR", ""),

		BadgeSigningKey: getEnv("BADGE_SIGNING_KEY", ""),
		BadgeKeyID:      getEnv("BADGE_KEY_ID", "mooc-badges-1"),
		IssuerName:      getEnv("ISSUER_NAME", "Plataforma MOOC"),

		SMTPHost: getEnv("SMTP_HOST", "localhost"),
		SMTPPort: getEnv("SMTP_PORT", "1025"),
		SMTPFrom: getEnv("SMTP_FROM", "no-reply@mooc.local"),

		SessionTTL:             getDuration("SESSION_TTL", 30*24*time.Hour),
		PublicBaseURL:          getEnv("PUBLIC_BASE_URL", "http://localhost:3000"),
		CookieSecure:           getBool("COOKIE_SECURE", false),
		AuthRateLimitPerMinute: getInt("AUTH_RATE_LIMIT_PER_MINUTE", 10),
		WorkerConcurrency:      getInt("WORKER_CONCURRENCY", 5),
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// getEnvDefinida distingue una variable ausente de una definida vacía, al
// contrario que getEnv. Solo tiene sentido donde el vacío significa algo.
func getEnvDefinida(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok {
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

// getInt lee un entero positivo del entorno. Un valor ausente, no numérico o
// no positivo cae al de por defecto: un límite de cero dejaría la plataforma
// sin poder autenticar a nadie, que es peor que ignorar la configuración.
func getInt(key string, fallback int) int {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		return fallback
	}
	return n
}
