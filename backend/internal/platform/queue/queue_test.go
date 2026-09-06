package queue

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"github.com/hibiken/asynq"
)

// registroEnMemoria captura las líneas emitidas por el manejador.
type registroEnMemoria struct{ b strings.Builder }

func (r *registroEnMemoria) logger() *slog.Logger {
	return slog.New(slog.NewTextHandler(&r.b, &slog.HandlerOptions{Level: slog.LevelDebug}))
}

func TestSeAlertaSoloCuandoElTrabajoAgotaSusReintentos(t *testing.T) {
	// El enunciado pide que la alerta se emita al agotar los reintentos, no en
	// cada fallo: alertar en los intermedios ahogaría la señal que importa.
	casos := []struct {
		nombre  string
		intento int
		maximo  int
		alerta  bool
	}{
		{"primer fallo", 0, 3, false},
		{"penúltimo fallo", 2, 3, false},
		{"último fallo", 3, 3, true},
	}

	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			var reg registroEnMemoria
			RegistrarIntentoFallido(context.Background(), reg.logger(), IntentoFallido{
				Tarea: TaskProcessMedia, ID: "tarea-1", Cola: "default",
				Intento: c.intento, Maximo: c.maximo, Err: errors.New("ffmpeg reventó"),
			})

			salida := reg.b.String()
			alerto := strings.Contains(salida, AccionAgotado)
			if alerto != c.alerta {
				t.Errorf("alerta=%v, se esperaba %v. Salida:\n%s", alerto, c.alerta, salida)
			}
			nivel := "level=WARN"
			if c.alerta {
				nivel = "level=ERROR"
			}
			if !strings.Contains(salida, nivel) {
				t.Errorf("se esperaba %s en la salida:\n%s", nivel, salida)
			}
			// La causa siempre debe quedar registrada, alerte o no.
			if !strings.Contains(salida, "ffmpeg") {
				t.Errorf("el error original debería registrarse:\n%s", salida)
			}
		})
	}
}

func TestElManejadorSeMontaSobreAsynqSinPanic(t *testing.T) {
	var reg registroEnMemoria
	manejador := NuevoManejadorDeErrores(reg.logger())
	// Un contexto sin metadatos de tarea no debe provocar un panic dentro del
	// worker: registrar un fallo nunca puede ser peor que el fallo mismo. Sin
	// metadatos, intento y máximo valen cero, así que cuenta como agotado.
	manejador.HandleError(context.Background(), asynq.NewTask(TaskProcessMedia, nil), errors.New("boom"))
	if !strings.Contains(reg.b.String(), AccionAgotado) {
		t.Errorf("debería haberse registrado la alerta:\n%s", reg.b.String())
	}
}

func TestReintentosMaximosSegunLaEspecificacion(t *testing.T) {
	if MaxRetry != 3 {
		t.Errorf("el enunciado fija tres reintentos antes de la DLQ, hay %d", MaxRetry)
	}
}
