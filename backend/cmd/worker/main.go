// Command worker consume los trabajos asíncronos (transcodificación,
// badges) publicados por la API a través de la cola en Redis, con
// reintentos con backoff y dead-letter queue gestionados por asynq.
package main

import (
	"context"
	"log"
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
	log.Printf("worker iniciado (env=%s, redis=%s)", cfg.Env, cfg.RedisAddr)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	pool, err := postgres.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("worker: postgres: %v", err)
	}
	defer pool.Close()

	storageClient, err := storage.New(ctx, storage.Config{
		Endpoint: cfg.S3Endpoint, AccessKey: cfg.S3AccessKey, SecretKey: cfg.S3SecretKey,
		UseSSL: cfg.S3UseSSL, Bucket: cfg.S3Bucket, PublicURL: cfg.S3PublicURL,
	})
	if err != nil {
		log.Fatalf("worker: storage: %v", err)
	}

	processor := &media.Processor{
		Storage: storageClient,
		Assets:  postgres.NewMediaRepo(pool),
		Courses: postgres.NewCourseRepo(pool),
	}

	mux := asynq.NewServeMux()
	mux.HandleFunc(queue.TaskProcessMedia, processor.HandleProcessMedia)

	srv := queue.NewServer(cfg.RedisAddr, 5)

	go func() {
		if err := srv.Run(mux); err != nil {
			log.Fatalf("worker: %v", err)
		}
	}()

	<-ctx.Done()
	srv.Shutdown()
	log.Println("worker detenido")
}
