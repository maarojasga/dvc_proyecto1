package queue

import (
	"context"
	"encoding/json"

	"github.com/hibiken/asynq"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

// Propagación de la traza a través de la cola.
//
// Una cola corta la traza: la API encola y termina, y el worker empieza de
// cero. Eso deja el flujo que más importa demostrar —subir un vídeo, verlo
// transcodificado— partido en dos trazas sin relación, y con los logs de cada
// pieza por separado no se puede reconstruir.
//
// Se resuelve llevando el contexto de traza dentro del propio trabajo: la API
// lo inyecta al encolar y el worker lo extrae al ejecutar. El transporte son
// las cabeceras estándar de W3C Trace Context, así que un colector cualquiera
// entiende el resultado sin configuración especial.

// Traza es el sobre que viaja con cada trabajo.
//
// Es un mapa y no un tipo con campos a propósito: el propagador decide qué
// claves usar, y fijarlas aquí ataría el formato.
type Traza map[string]string

// Set y Get hacen de Traza un carrier de OpenTelemetry.
func (t Traza) Set(clave, valor string) { t[clave] = valor }

func (t Traza) Get(clave string) string { return t[clave] }

func (t Traza) Keys() []string {
	claves := make([]string, 0, len(t))
	for k := range t {
		claves = append(claves, k)
	}
	return claves
}

// InyectarTraza mete el contexto de traza actual en el sobre.
func InyectarTraza(ctx context.Context) Traza {
	t := Traza{}
	otel.GetTextMapPropagator().Inject(ctx, t)
	return t
}

// ExtraerTraza devuelve un contexto que continúa la traza del sobre.
//
// Si el sobre viene vacío —un trabajo encolado antes de instrumentar, o por
// una versión anterior— devuelve el contexto tal cual: se pierde el enlace,
// no el trabajo.
func ExtraerTraza(ctx context.Context, t Traza) context.Context {
	if len(t) == 0 {
		return ctx
	}
	return otel.GetTextMapPropagator().Extract(ctx, t)
}

// interfaz mínima para que el envoltorio no dependa de un propagador concreto.
var _ propagation.TextMapCarrier = Traza{}

// trazaDelTrabajo saca el sobre del cuerpo del trabajo.
//
// Se lee con un struct parcial: cada tipo de trabajo tiene su propio cuerpo, y
// aquí solo interesa el sobre. Un cuerpo que no se pueda leer no es un
// problema de este envoltorio —lo detectará el manejador— así que se devuelve
// vacío y el trabajo sigue.
func trazaDelTrabajo(t *asynq.Task) Traza {
	var sobre struct {
		Traza Traza `json:"traza"`
	}
	if err := json.Unmarshal(t.Payload(), &sobre); err != nil {
		return nil
	}
	return sobre.Traza
}

// ConTraza envuelve un manejador de asynq para que su trabajo continúe la
// traza de quien lo encoló.
//
// El tramo se marca con error cuando el manejador falla, porque un trabajo que
// va a la DLQ tras tres intentos es exactamente lo que hay que poder encontrar
// después.
func ConTraza(nombre string, h asynq.HandlerFunc) asynq.HandlerFunc {
	trazador := otel.Tracer("mooc/worker")
	return func(ctx context.Context, t *asynq.Task) error {
		sobre := trazaDelTrabajo(t)
		ctx = ExtraerTraza(ctx, sobre)

		ctx, tramo := trazador.Start(ctx, nombre, trace.WithSpanKind(trace.SpanKindConsumer))
		defer tramo.End()

		tramo.SetAttributes(attribute.String("messaging.destination.name", t.Type()))
		if id, ok := asynq.GetTaskID(ctx); ok {
			// El id del trabajo es la clave de idempotencia: con él, en una
			// entrega duplicada se ve que los dos tramos son el mismo trabajo.
			tramo.SetAttributes(attribute.String("messaging.message.id", id))
		}
		if intento, ok := asynq.GetRetryCount(ctx); ok {
			tramo.SetAttributes(attribute.Int("messaging.retry_count", intento))
		}

		err := h(ctx, t)
		if err != nil {
			tramo.RecordError(err)
			tramo.SetStatus(codes.Error, err.Error())
		}
		return err
	}
}
