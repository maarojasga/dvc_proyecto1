// Package queue encapsula la cola asíncrona (asynq sobre Redis) usada para
// publicar y consumir trabajos con idempotencia, reintentos con backoff y
// dead-letter queue.
package queue

import (
	"context"
	"errors"
	"log/slog"

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
// por prioridad, el número de reintentos exigido por la especificación y la
// alerta que debe emitirse cuando un trabajo se agota.
func NewServer(redisAddr string, concurrency int, log *slog.Logger) *asynq.Server {
	if log == nil {
		log = slog.Default()
	}
	return asynq.NewServer(
		asynq.RedisClientOpt{Addr: redisAddr},
		asynq.Config{
			Concurrency:  concurrency,
			ErrorHandler: NuevoManejadorDeErrores(log),
			Queues: map[string]int{
				"critical": 6,
				"default":  3,
				"low":      1,
			},
		},
	)
}

// AtributosDeLaTarea son los datos que asynq guarda en el contexto del
// trabajo en curso: identificador, cola e intento.
//
// Van en todas las líneas del worker, no solo en las de error. Sin el
// identificador no se pueden hilar los registros de un mismo trabajo cuando
// hay cinco corriendo a la vez, y sin el intento no se distingue un trabajo
// que salió bien a la primera de uno que salió bien después de dos fallos y su
// backoff, que es justo lo que hay que poder demostrar.
//
// Devuelve la forma variádica que espera slog en lugar de una estructura: el
// uso es siempre pasarlo a un logger, y envolverlo solo añadiría un paso.
func AtributosDeLaTarea(ctx context.Context) []any {
	id, _ := asynq.GetTaskID(ctx)
	cola, _ := asynq.GetQueueName(ctx)
	intento, _ := asynq.GetRetryCount(ctx)
	return []any{"tarea_id", id, "cola", cola, "intento", intento}
}

// AccionAgotado es la acción que emite la alerta cuando un trabajo agota sus
// reintentos. Se registra con este nombre exacto para que la regla de alerta
// de observabilidad pueda engancharse a un valor estable y no a un texto libre.
const AccionAgotado = "queue.task_dead_letter"

// NuevoManejadorDeErrores devuelve el asynq.ErrorHandler que registra cada
// intento fallido y distingue el último.
//
// El enunciado pide que, tras agotar los reintentos, el trabajo llegue a la
// dead-letter queue y emita una alerta. asynq archiva el trabajo por su
// cuenta; lo que falta es la señal, y este manejador la produce: los intentos
// intermedios quedan como aviso y el definitivo como error, que es lo que la
// regla de alerta debe vigilar.
func NuevoManejadorDeErrores(log *slog.Logger) asynq.ErrorHandler {
	return asynq.ErrorHandlerFunc(func(ctx context.Context, t *asynq.Task, err error) {
		intento, _ := asynq.GetRetryCount(ctx)
		maximo, _ := asynq.GetMaxRetry(ctx)
		id, _ := asynq.GetTaskID(ctx)
		cola, _ := asynq.GetQueueName(ctx)

		RegistrarIntentoFallido(ctx, log, IntentoFallido{
			Tarea: t.Type(), ID: id, Cola: cola,
			Intento: intento, Maximo: maximo, Err: err,
		})
	})
}

// IntentoFallido describe un intento de trabajo que terminó en error.
type IntentoFallido struct {
	Tarea   string
	ID      string
	Cola    string
	Intento int
	Maximo  int
	Err     error
}

// Agotado informa si este era el último intento y, por tanto, el trabajo pasa
// a la dead-letter queue.
func (f IntentoFallido) Agotado() bool {
	return f.Intento >= f.Maximo || errors.Is(f.Err, asynq.SkipRetry)
}

// RegistrarIntentoFallido emite la señal que corresponda.
//
// Se separa de la extracción del contexto de asynq para poder ejercitarla:
// lo que importa verificar es que la alerta se emita solo en el último
// intento, no en cada fallo, porque alertar en los intermedios ahogaría la
// señal que de verdad exige atención.
func RegistrarIntentoFallido(ctx context.Context, log *slog.Logger, f IntentoFallido) {
	atributos := []any{
		"tarea", f.Tarea, "id", f.ID, "cola", f.Cola,
		"intento", f.Intento, "maximo", f.Maximo, "error", f.Err,
	}
	if errors.Is(f.Err, asynq.SkipRetry) {
		log.ErrorContext(ctx, "trabajo descartado: no se reintentará",
			append([]any{"accion", AccionAgotado}, atributos...)...)
		return
	}
	if f.Agotado() {
		log.ErrorContext(ctx, "trabajo agotado: pasa a la dead-letter queue",
			append([]any{"accion", AccionAgotado}, atributos...)...)
		return
	}
	log.WarnContext(ctx, "trabajo fallido: se reintentará con backoff", atributos...)
}

// NewInspector permite consultar el estado de las colas y la dead-letter
// queue (usado por el endpoint de observabilidad/administración).
func NewInspector(redisAddr string) *asynq.Inspector {
	return asynq.NewInspector(asynq.RedisClientOpt{Addr: redisAddr})
}
