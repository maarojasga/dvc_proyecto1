// Command worker consumirá los trabajos asíncronos (transcodificación, badges, etc.)
// publicados por la API a través de la cola en Redis.
package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/DES-SOLUCIONES-CLOUD/proyecto-1/backend/internal/config"
)

func main() {
	cfg := config.Load()
	log.Printf("worker iniciado (env=%s, redis=%s)", cfg.Env, cfg.RedisAddr)

	// TODO: registrar consumidores asynq (transcodificación HLS, PDFs, badges, DLQ).

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	log.Println("worker detenido")
}
