// Command api expone la API REST (/api/v1) del monolito modular.
package main

import (
	"context"
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
	coursesSvc := courses.NewService(courseRepo)
	enrollmentsSvc := enrollments.NewService(enrollmentRepo, courseRepo, progressRepo)
	progresoSvc := progreso.NewService(progressRepo, enrollmentRepo, courseRepo, quizRepo, badgeRepo, userRepo)
	// El servicio de quizzes avisa al de progreso al cerrar un intento, porque
	// aprobar una evaluacion puede ser lo ultimo que faltaba para el curso.
	quizzesSvc := quizzes.NewService(quizRepo, courseRepo, coursesSvc, enrollmentRepo, progresoSvc)

	if err := bootstrapAdmin(ctx, userRepo); err != nil {
		log.Printf("api: no se pudo crear el administrador inicial: %v", err)
	}

	router := httpserver.NewRouter(httpserver.Deps{
		Auth: authSvc, Admin: adminSvc, Courses: coursesSvc, Enrollments: enrollmentsSvc,
		Quizzes: quizzesSvc, Progreso: progresoSvc,
		Media: mediaRepo, Storage: storageClient, Entrega: storageClient,
		Redis: rdb, Queue: queueClient,
		Inspector:  queue.NewInspector(cfg.RedisAddr),
		CORSOrigin: cfg.PublicBaseURL, CookieSecure: cfg.CookieSecure,
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
