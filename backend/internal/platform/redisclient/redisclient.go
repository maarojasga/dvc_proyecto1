// Package redisclient provee el cliente Redis compartido para sesiones,
// caché y límites de tasa (la cola asíncrona la gestiona asynq por separado
// con su propia conexión, ver internal/platform/queue).
package redisclient

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// Connect crea el cliente y verifica conectividad con PING.
func Connect(ctx context.Context, addr string) (*redis.Client, error) {
	client := redis.NewClient(&redis.Options{Addr: addr})

	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := client.Ping(pingCtx).Err(); err != nil {
		return nil, fmt.Errorf("redis: ping falló: %w", err)
	}
	return client, nil
}
