package queue

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/hibiken/asynq"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
)

// instrumentar deja un proveedor de trazas que recuerda lo que se emitió, y el
// propagador W3C instalado, igual que en producción.
func instrumentar(t *testing.T) *tracetest.SpanRecorder {
	t.Helper()
	grabadora := tracetest.NewSpanRecorder()
	prov := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(grabadora))
	previoProv := otel.GetTracerProvider()
	previoProp := otel.GetTextMapPropagator()
	otel.SetTracerProvider(prov)
	otel.SetTextMapPropagator(propagation.TraceContext{})
	t.Cleanup(func() {
		otel.SetTracerProvider(previoProv)
		otel.SetTextMapPropagator(previoProp)
	})
	return grabadora
}

// Lo que de verdad importa: el trabajo del worker tiene que quedar en la misma
// traza que la petición que lo encoló. Sin esto, subir un vídeo y verlo
// transcodificado son dos trazas sueltas y el flujo no se puede seguir.
func TestElTrabajoContinuaLaTrazaDeQuienLoEncolo(t *testing.T) {
	grabadora := instrumentar(t)

	// La API abre un tramo y encola dentro de él.
	ctxAPI, tramoAPI := otel.Tracer("prueba").Start(context.Background(), "POST /upload")
	trazaEsperada := tramoAPI.SpanContext().TraceID()
	sobre := InyectarTraza(ctxAPI)
	tramoAPI.End()

	if len(sobre) == 0 {
		t.Fatal("el sobre salió vacío; la traza no viaja con el trabajo")
	}

	cuerpo, err := json.Marshal(MediaProcessPayload{Kind: "video", Traza: sobre})
	if err != nil {
		t.Fatalf("serializar: %v", err)
	}
	tarea := asynq.NewTask(TaskProcessMedia, cuerpo)

	// El worker ejecuta en un contexto nuevo, como de verdad.
	var trazaEnElWorker trace.TraceID
	manejador := ConTraza("procesar multimedia", func(ctx context.Context, _ *asynq.Task) error {
		trazaEnElWorker = trace.SpanContextFromContext(ctx).TraceID()
		return nil
	})
	if err := manejador(context.Background(), tarea); err != nil {
		t.Fatalf("manejador: %v", err)
	}

	if trazaEnElWorker != trazaEsperada {
		t.Fatalf("el worker quedó en la traza %s y la API en %s", trazaEnElWorker, trazaEsperada)
	}

	// Y el tramo del worker cuelga de la traza, no es una raíz nueva.
	tramos := grabadora.Ended()
	var delWorker sdktrace.ReadOnlySpan
	for _, s := range tramos {
		if s.Name() == "procesar multimedia" {
			delWorker = s
		}
	}
	if delWorker == nil {
		t.Fatal("no se emitió el tramo del worker")
	}
	if !delWorker.Parent().IsValid() {
		t.Error("el tramo del worker no tiene padre: es una traza nueva, no una continuación")
	}
	if delWorker.SpanKind() != trace.SpanKindConsumer {
		t.Errorf("el tramo del worker es %v, esperaba consumidor", delWorker.SpanKind())
	}
}

// Un trabajo fallido tiene que verse como fallido: es lo que hay que encontrar
// cuando algo acabó en la dead-letter queue.
func TestUnTrabajoFallidoMarcaElTramo(t *testing.T) {
	grabadora := instrumentar(t)
	falla := errors.New("no se pudo transcodificar")

	tarea := asynq.NewTask(TaskProcessMedia, []byte(`{"kind":"video"}`))
	manejador := ConTraza("procesar multimedia", func(context.Context, *asynq.Task) error { return falla })
	if err := manejador(context.Background(), tarea); !errors.Is(err, falla) {
		t.Fatalf("el envoltorio se comió el error: %v", err)
	}

	tramos := grabadora.Ended()
	if len(tramos) != 1 {
		t.Fatalf("se emitieron %d tramos", len(tramos))
	}
	if tramos[0].Status().Code.String() != "Error" {
		t.Errorf("el tramo quedó en %s, esperaba Error", tramos[0].Status().Code)
	}
	if len(tramos[0].Events()) == 0 {
		t.Error("el tramo no registró el error como evento, así que no se puede leer el motivo")
	}
}

// Un trabajo sin sobre —encolado por una versión anterior— tiene que
// ejecutarse igual. Se pierde el enlace de la traza, no el trabajo.
func TestUnTrabajoSinTrazaSeEjecutaIgual(t *testing.T) {
	instrumentar(t)
	ejecutado := false
	tarea := asynq.NewTask(TaskProcessMedia, []byte(`{"kind":"audio"}`))
	manejador := ConTraza("procesar multimedia", func(context.Context, *asynq.Task) error {
		ejecutado = true
		return nil
	})
	if err := manejador(context.Background(), tarea); err != nil {
		t.Fatalf("manejador: %v", err)
	}
	if !ejecutado {
		t.Fatal("el trabajo no se ejecutó")
	}
}

// Un cuerpo ilegible no es problema de este envoltorio: lo detecta el
// manejador, que es quien sabe qué esperaba.
func TestUnCuerpoIlegibleNoRompeElEnvoltorio(t *testing.T) {
	instrumentar(t)
	llamado := false
	tarea := asynq.NewTask(TaskProcessMedia, []byte(`{no es json`))
	manejador := ConTraza("procesar multimedia", func(context.Context, *asynq.Task) error {
		llamado = true
		return nil
	})
	if err := manejador(context.Background(), tarea); err != nil {
		t.Fatalf("manejador: %v", err)
	}
	if !llamado {
		t.Fatal("el envoltorio no llamó al manejador")
	}
}
