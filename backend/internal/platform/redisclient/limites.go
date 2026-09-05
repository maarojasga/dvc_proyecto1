package redisclient

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

// LimitadorTasa implementa un contador con ventana fija sobre Redis.
//
// Al vivir en Redis y no en memoria del proceso, el limite es del servicio
// completo y no de cada instancia: la API sigue siendo sin estado y escalar a
// N replicas no multiplica por N el cupo del atacante.
type LimitadorTasa struct{ c *redis.Client }

// NuevoLimitadorTasa construye el limitador.
func NuevoLimitadorTasa(c *redis.Client) *LimitadorTasa {
	return &LimitadorTasa{c: c}
}

// Resultado describe el estado del cupo tras contar una peticion.
type Resultado struct {
	Permitido bool
	Restantes int
	Reintento time.Duration
}

// Permitir cuenta una peticion contra la clave y dice si cabe en el cupo.
//
// Ante un fallo de Redis se permite la peticion: quedarse sin limitador
// degrada la proteccion, pero caerse entero deja fuera a los usuarios
// legitimos.
func (l *LimitadorTasa) Permitir(ctx context.Context, clave string, maximo int, ventana time.Duration) Resultado {
	clave = "limite:" + clave
	tuberia := l.c.TxPipeline()
	incremento := tuberia.Incr(ctx, clave)
	tuberia.ExpireNX(ctx, clave, ventana)
	if _, err := tuberia.Exec(ctx); err != nil {
		return Resultado{Permitido: true, Restantes: maximo}
	}

	usadas := int(incremento.Val())
	if usadas > maximo {
		ttl, err := l.c.TTL(ctx, clave).Result()
		if err != nil || ttl < 0 {
			ttl = ventana
		}
		return Resultado{Permitido: false, Restantes: 0, Reintento: ttl}
	}
	return Resultado{Permitido: true, Restantes: maximo - usadas}
}
