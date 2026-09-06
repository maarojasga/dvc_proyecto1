// Command worker consume los trabajos asíncronos (transcodificación,
// badges) publicados por la API a través de la cola en Redis, con
// reintentos con backoff y dead-letter queue gestionados por asynq.
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/hibiken/asynq"

	"github.com/DES-SOLUCIONES-CLOUD/proyecto-1/backend/internal/config"
	"github.com/DES-SOLUCIONES-CLOUD/proyecto-1/backend/internal/platform/media"
	"github.com/DES-SOLUCIONES-CLOUD/proyecto-1/backend/internal/platform/postgres"
	"github.com/DES-SOLUCIONES-CLOUD/proyecto-1/backend/internal/platform/queue"
	"github.com/DES-SOLUCIONES-CLOUD/proyecto-1/backend/internal/platform/storage"
)

func main() {
	cfg := config.Load()
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(log)
	log.Info("worker iniciado", "env", cfg.Env, "redis", cfg.RedisAddr)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	pool, err := postgres.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Error("worker: postgres", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	storageClient, err := storage.New(ctx, storage.Config{
		Endpoint: cfg.S3Endpoint, AccessKey: cfg.S3AccessKey, SecretKey: cfg.S3SecretKey,
		UseSSL: cfg.S3UseSSL, Bucket: cfg.S3Bucket, Region: cfg.S3Region, PublicURL: cfg.S3PublicURL,
		PublicEndpoint: cfg.S3PublicEndpoint, PublicUseSSL: cfg.S3PublicUseSSL,
	})
	if err != nil {
		log.Error("worker: storage", "error", err)
		os.Exit(1)
	}

	processor := &media.Processor{
		Storage: storageClient,
		Assets:  postgres.NewMediaRepo(pool),
		Courses: postgres.NewCourseRepo(pool),
		Log:     log,
	}

	mux := asynq.NewServeMux()
	mux.HandleFunc(queue.TaskProcessMedia, processor.HandleProcessMedia)

	srv := queue.NewServer(cfg.RedisAddr, 5, log)

	go func() {
		if err := srv.Run(mux); err != nil {
			log.Error("worker: el servidor de la cola termino con error", "error", err)
			os.Exit(1)
		}
	}()

	<-ctx.Done()
	srv.Shutdown()
	log.Info("worker detenido")
}
