package queue

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/hibiken/asynq"
)

// Trabajo describe algo que hay que procesar fuera de la petición HTTP.
//
// Es el vocabulario propio de la aplicación, no el del broker: antes los
// manejadores construían un *asynq.Task y llamaban a Enqueue con las opciones
// del SDK, de modo que la capa de entrega dependía de la librería de colas y
// no había manera de comprobar en una prueba qué se había encolado sin
// levantar Redis.
type Trabajo struct {
	// Tipo es el nombre del trabajo (queue.TaskProcessMedia y compañía).
	Tipo string
	// ClaveDeIdempotencia identifica el trabajo. Dos publicaciones con la
	// misma clave son la misma: es lo que evita que un reintento del cliente
	// dispare dos transcodificaciones.
	ClaveDeIdempotencia string
	// Payload se serializa a JSON.
	Payload any
	// MaxIntentos, si es cero, usa MaxRetry.
	MaxIntentos int
}

// Encolador publica trabajos.
type Encolador interface {
	Encolar(ctx context.Context, t Trabajo) error
}

// ClienteAsynq adapta el cliente de asynq a Encolador.
type ClienteAsynq struct{ c *asynq.Client }

// NuevoEncolador envuelve un cliente de asynq.
func NuevoEncolador(c *asynq.Client) *ClienteAsynq { return &ClienteAsynq{c: c} }

func (e *ClienteAsynq) Encolar(ctx context.Context, t Trabajo) error {
	cuerpo, err := json.Marshal(t.Payload)
	if err != nil {
		return fmt.Errorf("queue: no se pudo serializar el trabajo %s: %w", t.Tipo, err)
	}
	intentos := t.MaxIntentos
	if intentos <= 0 {
		intentos = MaxRetry
	}
	opciones := []asynq.Option{asynq.MaxRetry(intentos)}
	if t.ClaveDeIdempotencia != "" {
		opciones = append(opciones, asynq.TaskID(t.ClaveDeIdempotencia))
	}
	if _, err := e.c.EnqueueContext(ctx, asynq.NewTask(t.Tipo, cuerpo), opciones...); err != nil {
		if errors.Is(err, asynq.ErrTaskIDConflict) || errors.Is(err, asynq.ErrDuplicateTask) {
			// La misma confirmación llegó dos veces: el trabajo ya está.
			return nil
		}
		return fmt.Errorf("queue: no se pudo encolar %s: %w", t.Tipo, err)
	}
	return nil
}
