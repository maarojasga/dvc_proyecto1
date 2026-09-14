// Command api expone la API REST (/api/v1) del monolito modular.
package main

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/google/uuid"

	"github.com/DES-SOLUCIONES-CLOUD/proyecto-1/backend/internal/app/admin"
	"github.com/DES-SOLUCIONES-CLOUD/proyecto-1/backend/internal/app/auth"
	"github.com/DES-SOLUCIONES-CLOUD/proyecto-1/backend/internal/app/courses"
	"github.com/DES-SOLUCIONES-CLOUD/proyecto-1/backend/internal/app/enrollments"
	"github.com/DES-SOLUCIONES-CLOUD/proyecto-1/backend/internal/app/progreso"
	"github.com/DES-SOLUCIONES-CLOUD/proyecto-1/backend/internal/app/quizzes"
	"github.com/DES-SOLUCIONES-CLOUD/proyecto-1/backend/internal/config"
	"github.com/DES-SOLUCIONES-CLOUD/proyecto-1/backend/internal/domain/user"
	"github.com/DES-SOLUCIONES-CLOUD/proyecto-1/backend/internal/platform/antimalware"
	"github.com/DES-SOLUCIONES-CLOUD/proyecto-1/backend/internal/platform/httpserver"
	"github.com/DES-SOLUCIONES-CLOUD/proyecto-1/backend/internal/platform/mailer"
	"github.com/DES-SOLUCIONES-CLOUD/proyecto-1/backend/internal/platform/postgres"
	"github.com/DES-SOLUCIONES-CLOUD/proyecto-1/backend/internal/platform/queue"
	"github.com/DES-SOLUCIONES-CLOUD/proyecto-1/backend/internal/platform/redisclient"
	"github.com/DES-SOLUCIONES-CLOUD/proyecto-1/backend/internal/platform/security"
	"github.com/DES-SOLUCIONES-CLOUD/proyecto-1/backend/internal/platform/storage"

	"github.com/DES-SOLUCIONES-CLOUD/proyecto-1/backend/migrations"
)

func main() {
	cfg := config.Load()
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	pool, err := postgres.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("api: postgres: %v", err)
	}
	defer pool.Close()

	if err := postgres.Migrate(ctx, pool, migrations.FS); err != nil {
		log.Fatalf("api: migraciones: %v", err)
	}

	rdb, err := redisclient.Connect(ctx, cfg.RedisAddr)
	if err != nil {
		log.Fatalf("api: redis: %v", err)
	}
	defer rdb.Close()

	storageClient, err := storage.New(ctx, storage.Config{
		Endpoint: cfg.S3Endpoint, AccessKey: cfg.S3AccessKey, SecretKey: cfg.S3SecretKey,
		UseSSL: cfg.S3UseSSL, Bucket: cfg.S3Bucket, Region: cfg.S3Region, PublicURL: cfg.S3PublicURL,
		PublicEndpoint: cfg.S3PublicEndpoint, PublicUseSSL: cfg.S3PublicUseSSL,
	})
	if err != nil {
		log.Fatalf("api: storage: %v", err)
	}

	m := mailer.New(cfg.SMTPHost, cfg.SMTPPort, cfg.SMTPFrom)
	queueClient := queue.NewClient(cfg.RedisAddr)
	defer queueClient.Close()

	userRepo := postgres.NewUserRepo(pool)
	courseRepo := postgres.NewCourseRepo(pool)
	enrollmentRepo := postgres.NewEnrollmentRepo(pool)
	mediaRepo := postgres.NewMediaRepo(pool)
	progressRepo := postgres.NewProgressRepo(pool)
	quizRepo := postgres.NewQuizRepo(pool)
	badgeRepo := postgres.NewBadgeRepo(pool)

	authSvc := auth.NewService(userRepo, m, cfg.PublicBaseURL, cfg.SessionTTL)
	adminSvc := admin.NewService(userRepo)
	iframeRepo := postgres.NewIframeRepo(pool)
	colaboradoresRepo := postgres.NewColaboradoresRepo(pool)
	coursesSvc := courses.NewService(courseRepo, iframeRepo, colaboradoresRepo)
	enrollmentsSvc := enrollments.NewService(enrollmentRepo, courseRepo, progressRepo, iframeRepo)
	progresoSvc := progreso.NewService(progressRepo, enrollmentRepo, courseRepo, quizRepo, badgeRepo, storageClient, userRepo)
	// El servicio de quizzes avisa al de progreso al cerrar un intento, porque
	// aprobar una evaluacion puede ser lo ultimo que faltaba para el curso.
	quizzesSvc := quizzes.NewService(quizRepo, courseRepo, coursesSvc, enrollmentRepo, progresoSvc)

	if err := bootstrapAdmin(ctx, userRepo); err != nil {
		log.Printf("api: no se pudo crear el administrador inicial: %v", err)
	}

	escaner := antimalware.Nuevo(cfg.ClamAVAddr)
	if cfg.ClamAVAddr == "" {
		log.Printf("api: antimalware integrado (sin CLAMAV_ADDR configurado)")
	} else {
		log.Printf("api: antimalware con clamd en %s", cfg.ClamAVAddr)
	}

	router := httpserver.NewRouter(httpserver.Deps{
		Auth: authSvc, Admin: adminSvc, Courses: coursesSvc, Enrollments: enrollmentsSvc,
		Quizzes: quizzesSvc, Progreso: progresoSvc,
		Media: mediaRepo, Storage: storageClient, Entrega: storageClient,
		Antimalware:   escaner,
		Iframes:       iframeRepo,
		Metricas:      postgres.NewMetricasRepo(pool),
		Revisiones:    postgres.NewRevisionesRepo(pool),
		Colaboradores: colaboradoresRepo,
		Subtitulos:    postgres.NewSubtitulosRepo(pool),
		Foros:         postgres.NewForosRepo(pool),
		Credenciales:  emisorDeCredenciales(cfg),
		Exportacion:   postgres.NewExportacionRepo(pool),
		Auditor:       userRepo,
		Redis:         rdb, Queue: queue.NuevoEncolador(queueClient),
		Inspector:  queue.NewInspector(cfg.RedisAddr),
		CORSOrigin: cfg.PublicBaseURL, CookieSecure: cfg.CookieSecure,
		AuthRateLimitPerMinute: cfg.AuthRateLimitPerMinute,
	})

	srv := &http.Server{
		Addr:              ":" + cfg.HTTPPort,
		Handler:           router,
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		log.Printf("api escuchando en :%s (env=%s)", cfg.HTTPPort, cfg.Env)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("api: %v", err)
		}
	}()

	<-ctx.Done()
	log.Println("api: apagando...")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("api: error durante el apagado: %v", err)
	}
}

// bootstrapAdmin crea el primer administrador a partir de ADMIN_EMAIL /
// ADMIN_PASSWORD si no existe ningún administrador aún. Solo aplica en el
// primer arranque del entorno; en producción las variables deben retirarse
// después de usarse.
func bootstrapAdmin(ctx context.Context, users *postgres.UserRepo) error {
	email := os.Getenv("ADMIN_EMAIL")
	password := os.Getenv("ADMIN_PASSWORD")
	if email == "" || password == "" {
		return nil
	}
	if _, err := users.GetByEmail(ctx, email); err == nil {
		return nil // ya existe
	} else if !errors.Is(err, postgres.ErrNotFound) {
		return err
	}

	hash, err := security.HashPassword(password)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	admin := &user.User{
		ID: uuid.New(), Email: email, PasswordHash: hash, FullName: "Administrador",
		Role: user.RoleAdmin, Status: user.StatusActive, EmailVerifiedAt: &now,
		CreatedAt: now, UpdatedAt: now,
	}
	if err := users.Create(ctx, admin); err != nil {
		return err
	}
	log.Printf("api: administrador inicial creado (%s)", email)
	return nil
}

// emisorDeCredenciales prepara la firma de las credenciales Open Badges 3.0.
//
// Sin BADGE_SIGNING_KEY la plataforma arranca igual: las insignias siguen
// siendo verificables por su URL pública y lo único que no se puede ofrecer es
// la credencial portátil. Se avisa en el arranque porque, si alguien esperaba
// esa función, el fallo se descubriría en la pantalla de un estudiante.
func emisorDeCredenciales(cfg config.Config) httpserver.EmisorDeCredenciales {
	emisor := httpserver.EmisorDeCredenciales{
		KeyID:   cfg.BadgeKeyID,
		Nombre:  cfg.IssuerName,
		BaseURL: cfg.PublicBaseURL,
	}
	if cfg.BadgeSigningKey == "" {
		log.Printf("api: sin BADGE_SIGNING_KEY; las insignias no se emitirán como credencial firmada")
		return emisor
	}

	crudo, err := base64.StdEncoding.DecodeString(cfg.BadgeSigningKey)
	if err != nil || len(crudo) != ed25519.PrivateKeySize {
		// No se genera una clave al vuelo como sustituto: cada reinicio
		// produciría otra y las credenciales firmadas antes dejarían de
		// verificarse, que es peor que no firmarlas.
		log.Printf("api: BADGE_SIGNING_KEY no es una clave Ed25519 válida en base64; no se firmarán credenciales")
		return emisor
	}
	emisor.Privada = ed25519.PrivateKey(crudo)
	emisor.Publica = emisor.Privada.Public().(ed25519.PublicKey)
	log.Printf("api: credenciales Open Badges firmadas con la clave %q", emisor.KeyID)
	return emisor
}
