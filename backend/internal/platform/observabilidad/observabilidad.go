// Package observabilidad arranca la instrumentación OpenTelemetry de la API y
// de los workers: trazas y métricas con la misma identidad de servicio.
//
// La restricción técnica pide OpenTelemetry, y la razón práctica es la
// demostración: un flujo que cruza la API, Redis y un worker no se puede
// explicar con los logs de cada pieza por separado. Con una traza, la subida
// de un vídeo y su transcodificación son el mismo hilo.
package observabilidad

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

// Config es lo que hace falta para instrumentar.
type Config struct {
	// Servicio distingue la API de los workers en la misma traza.
	Servicio string
	// Version y Entorno acompañan a cada señal, para no confundir lo que pasa
	// en producción con lo que pasa en una prueba.
	Version string
	Entorno string
	// Endpoint es el colector OTLP (host:puerto, HTTP). Vacío desactiva la
	// exportación: en local no hay colector, y un arranque que falla por eso
	// sería peor que no tener trazas.
	Endpoint string
	// Muestreo es la fracción de trazas que se conservan, de 0 a 1. Con 2.000
	// usuarios concurrentes, guardar todas cuesta más que lo que informan.
	Muestreo float64
}

// Apagar cierra la instrumentación, vaciando lo que quede pendiente.
type Apagar func(context.Context) error

// Iniciar pone en marcha trazas y métricas.
//
// Devuelve una función de apagado que hay que llamar al terminar: sin ella se
// pierde lo último que se midió, que suele ser justo lo que se quería ver
// cuando algo falló al cerrar.
func Iniciar(ctx context.Context, cfg Config) (Apagar, error) {
	// El propagador se instala siempre, también sin colector. Es lo que hace
	// que el contexto de traza viaje entre la API y los workers, y cuesta
	// nada: si nadie exporta, no se pierde nada por tenerlo puesto.
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{}, propagation.Baggage{},
	))

	if cfg.Endpoint == "" {
		log.Printf("observabilidad: sin OTEL_EXPORTER_OTLP_ENDPOINT; trazas y métricas no se exportan")
		return func(context.Context) error { return nil }, nil
	}

	res, err := recurso(ctx, cfg)
	if err != nil {
		return nil, err
	}

	expTrazas, err := otlptracehttp.New(ctx,
		otlptracehttp.WithEndpoint(cfg.Endpoint),
		// Sin TLS hacia el colector: en este despliegue va en la red interna.
		// Hacia fuera iría con TLS, que es lo que exige la restricción.
		otlptracehttp.WithInsecure(),
	)
	if err != nil {
		return nil, fmt.Errorf("observabilidad: exportador de trazas: %w", err)
	}

	muestreo := cfg.Muestreo
	if muestreo <= 0 || muestreo > 1 {
		muestreo = 1
	}
	provTrazas := sdktrace.NewTracerProvider(
		sdktrace.WithResource(res),
		sdktrace.WithBatcher(expTrazas),
		// ParentBased: si la API decidió muestrear una petición, el worker que
		// continúa ese trabajo conserva la decisión. Si cada pieza decidiera
		// por su cuenta, quedarían trazas a medias, que es lo peor de los dos
		// mundos: se paga el coste y no se puede seguir el hilo.
		sdktrace.WithSampler(sdktrace.ParentBased(sdktrace.TraceIDRatioBased(muestreo))),
	)
	otel.SetTracerProvider(provTrazas)

	expMetricas, err := otlpmetrichttp.New(ctx,
		otlpmetrichttp.WithEndpoint(cfg.Endpoint),
		otlpmetrichttp.WithInsecure(),
	)
	if err != nil {
		return nil, fmt.Errorf("observabilidad: exportador de métricas: %w", err)
	}
	provMetricas := sdkmetric.NewMeterProvider(
		sdkmetric.WithResource(res),
		sdkmetric.WithReader(sdkmetric.NewPeriodicReader(expMetricas,
			sdkmetric.WithInterval(30*time.Second))),
	)
	otel.SetMeterProvider(provMetricas)

	log.Printf("observabilidad: exportando a %s (servicio=%s muestreo=%.2f)", cfg.Endpoint, cfg.Servicio, muestreo)

	return func(ctx context.Context) error {
		// Se apagan los dos aunque el primero falle: dejar el otro sin vaciar
		// perdería medidas por un error ajeno.
		return errors.Join(provTrazas.Shutdown(ctx), provMetricas.Shutdown(ctx))
	}, nil
}

func recurso(ctx context.Context, cfg Config) (*resource.Resource, error) {
	servicio := cfg.Servicio
	if servicio == "" {
		servicio = "mooc"
	}
	atributos := []attribute.KeyValue{semconv.ServiceName(servicio)}
	if cfg.Version != "" {
		atributos = append(atributos, semconv.ServiceVersion(cfg.Version))
	}
	if cfg.Entorno != "" {
		atributos = append(atributos, semconv.DeploymentEnvironment(cfg.Entorno))
	}
	res, err := resource.New(ctx,
		// FromEnv permite añadir atributos por OTEL_RESOURCE_ATTRIBUTES sin
		// tocar código, que es como se distingue una instancia de otra al
		// escalar.
		resource.WithFromEnv(),
		resource.WithHost(),
		resource.WithAttributes(atributos...),
	)
	if err != nil {
		// Un atributo de entorno mal escrito no debe impedir arrancar: se
		// pierde precisión en las señales, no el servicio.
		if errors.Is(err, resource.ErrPartialResource) || errors.Is(err, resource.ErrSchemaURLConflict) {
			log.Printf("observabilidad: recurso parcial: %v", err)
			return res, nil
		}
		return nil, fmt.Errorf("observabilidad: recurso: %w", err)
	}
	return res, nil
}
