// Package redisclient centraliza la conexion a Redis usada para sesiones,
// cache, limites de tasa e idempotencia.
package redisclient

import (
	"context"
	"fmt"

	"github.com/redis/go-redis/v9"
)

// Abrir crea el cliente y comprueba que Redis responde.
func Abrir(ctx context.Context, direccion string) (*redis.Client, error) {
	c := redis.NewClient(&redis.Options{Addr: direccion})
	if err := c.Ping(ctx).Err(); err != nil {
		_ = c.Close()
		return nil, fmt.Errorf("redis: sin conexion en %s: %w", direccion, err)
	}
	return c, nil
}
