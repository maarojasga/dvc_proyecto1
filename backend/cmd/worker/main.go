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
	"time"

	"github.com/hibiken/asynq"

	"github.com/DES-SOLUCIONES-CLOUD/proyecto-1/backend/internal/config"
	"github.com/DES-SOLUCIONES-CLOUD/proyecto-1/backend/internal/platform/media"
	"github.com/DES-SOLUCIONES-CLOUD/proyecto-1/backend/internal/platform/observabilidad"
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

	apagarObs, err := observabilidad.Iniciar(ctx, observabilidad.Config{
		Servicio: "mooc-worker", Version: cfg.AppVersion, Entorno: cfg.Env,
		Endpoint: cfg.OTLPEndpoint, Muestreo: cfg.OTLPMuestreo,
	})
	if err != nil {
		log.Error("worker: observabilidad", "error", err)
		os.Exit(1)
	}
	defer func() {
		// Contexto propio: el de arriba está cancelado tras la señal de
		// apagado, y con él no se vaciaría lo último medido.
		cierre, cancelar := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancelar()
		if err := apagarObs(cierre); err != nil {
			log.Error("worker: al cerrar la observabilidad", "error", err)
		}
	}()

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
	// ConTraza envuelve el manejador para que el trabajo continúe la traza de
	// quien lo encoló y el tramo quede marcado con error si falla, que es lo
	// que hay que poder encontrar cuando un trabajo acaba en la DLQ.
	mux.Handle(queue.TaskProcessMedia, queue.ConTraza("procesar multimedia", processor.HandleProcessMedia))

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
