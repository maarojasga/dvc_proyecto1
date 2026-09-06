// Package queue encapsula la cola asíncrona (asynq sobre Redis) usada para
// publicar y consumir trabajos con idempotencia, reintentos con backoff y
// dead-letter queue.
package queue

import (
	"github.com/hibiken/asynq"
)

const (
	TaskProcessMedia    = "media:process"
	TaskConvertDocument = "document:convert"
	TaskIssueBadge      = "badge:issue"
)

// MaxRetry es el número de reintentos antes de que un trabajo se considere
// agotado y pase al conjunto de tareas muertas (dead-letter) de asynq.
const MaxRetry = 3

// NewClient crea el cliente usado por la API para encolar trabajos.
func NewClient(redisAddr string) *asynq.Client {
	return asynq.NewClient(asynq.RedisClientOpt{Addr: redisAddr})
}

// NewServer crea el servidor consumido por los workers, con colas separadas
// por prioridad y el número de reintentos exigido por la especificación.
func NewServer(redisAddr string, concurrency int) *asynq.Server {
	return asynq.NewServer(
		asynq.RedisClientOpt{Addr: redisAddr},
		asynq.Config{
			Concurrency: concurrency,
			Queues: map[string]int{
				"critical": 6,
				"default":  3,
				"low":      1,
			},
		},
	)
}

// NewInspector permite consultar el estado de las colas y la dead-letter
// queue (usado por el endpoint de observabilidad/administración).
func NewInspector(redisAddr string) *asynq.Inspector {
	return asynq.NewInspector(asynq.RedisClientOpt{Addr: redisAddr})
}
