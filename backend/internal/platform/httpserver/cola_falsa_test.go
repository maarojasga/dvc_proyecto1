package httpserver_test

import (
	"context"
	"sync"

	"github.com/DES-SOLUCIONES-CLOUD/proyecto-1/backend/internal/platform/queue"
)

// colaFalsa registra los trabajos publicados en lugar de mandarlos a Redis.
//
// Sirve para dos cosas: que las pruebas de ingesta no necesiten la cola
// levantada, y que puedan comprobar qué se encoló —que una presentación va a
// convertirse y un vídeo a transcodificarse no se ve en la respuesta HTTP, que
// en ambos casos es un 202 idéntico.
type colaFalsa struct {
	mu        sync.Mutex
	trabajos  []queue.Trabajo
	fallarCon error
}

func nuevaColaFalsa() *colaFalsa { return &colaFalsa{} }

func (c *colaFalsa) Encolar(_ context.Context, t queue.Trabajo) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.fallarCon != nil {
		return c.fallarCon
	}
	c.trabajos = append(c.trabajos, t)
	return nil
}

// publicados devuelve una copia de lo encolado hasta ahora.
func (c *colaFalsa) publicados() []queue.Trabajo {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]queue.Trabajo(nil), c.trabajos...)
}

// ultimo devuelve el último trabajo publicado, o el cero si no hubo ninguno.
func (c *colaFalsa) ultimo() queue.Trabajo {
	pubs := c.publicados()
	if len(pubs) == 0 {
		return queue.Trabajo{}
	}
	return pubs[len(pubs)-1]
}
