// Command api expone la API REST (/api/v1) del monolito modular.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/DES-SOLUCIONES-CLOUD/proyecto-1/backend/internal/config"
	"github.com/DES-SOLUCIONES-CLOUD/proyecto-1/backend/internal/domain/user"
	"github.com/DES-SOLUCIONES-CLOUD/proyecto-1/backend/internal/platform/httpserver"
	"github.com/DES-SOLUCIONES-CLOUD/proyecto-1/backend/internal/platform/mailer"
	"github.com/DES-SOLUCIONES-CLOUD/proyecto-1/backend/internal/platform/passwords"
	"github.com/DES-SOLUCIONES-CLOUD/proyecto-1/backend/internal/platform/postgres"
	"github.com/DES-SOLUCIONES-CLOUD/proyecto-1/backend/internal/platform/redisclient"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(log)

	if err := ejecutar(log); err != nil {
		log.Error("la api termino con error", "error", err)
		os.Exit(1)
	}
}

func ejecutar(log *slog.Logger) error {
	cfg := config.Load()

	// El arranque tiene su propio plazo: si la base o Redis no responden, es
	// mejor fallar rapido y que el orquestador reintente.
	ctxArranque, cancelar := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancelar()

	pool, err := postgres.Abrir(ctxArranque, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	if err := postgres.Migrar(ctxArranque, pool, log); err != nil {
		return err
	}

	redis, err := redisclient.Abrir(ctxArranque, cfg.RedisAddr)
	if err != nil {
		return err
	}
	defer func() { _ = redis.Close() }()

	var notificador user.Notificador = mailer.Nuevo(mailer.Config{
		Direccion:   cfg.SMTPDireccion,
		Remitente:   cfg.SMTPRemitente,
		Usuario:     cfg.SMTPUsuario,
		Clave:       cfg.SMTPClave,
		URLFrontend: cfg.URLFrontend,
	}, log)
	if cfg.SMTPDireccion == "" {
		notificador = mailer.NuevoRegistro(log)
	}

	servicio, err := user.NuevoServicio(user.Dependencias{
		Usuarios:  postgres.NuevoRepositorioUsuarios(pool),
		Sesiones:  postgres.NuevoRepositorioSesiones(pool),
		Tokens:    postgres.NuevoRepositorioTokens(pool),
		Auditoria: postgres.NuevoAuditor(pool, log),
		Claves:    passwords.Nuevo(passwords.ParametrosPorDefecto()),
		Correos:   notificador,
		Cache:     redisclient.NuevaCacheSesiones(redis, log),
	})
	if err != nil {
		return err
	}

	api := httpserver.Nuevo(httpserver.Opciones{
		Usuarios:     servicio,
		Log:          log,
		Limitador:    redisclient.NuevoLimitadorTasa(redis),
		Idempotencia: redisclient.NuevoAlmacenIdempotencia(redis),
		CookieSegura: cfg.CookieSegura,
	})

	srv := &http.Server{
		Addr:              ":" + cfg.HTTPPort,
		Handler:           api.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       2 * time.Minute,
	}

	// Apagado ordenado: al recibir la senal se dejan de aceptar conexiones y
	// se espera a que terminen las que estan en curso, para que reemplazar
	// una instancia no corte peticiones a medias.
	ctxSenal, detener := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer detener()

	errores := make(chan error, 1)
	go func() {
		log.Info("api escuchando", "puerto", cfg.HTTPPort, "env", cfg.Env)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errores <- err
		}
	}()

	select {
	case err := <-errores:
		return err
	case <-ctxSenal.Done():
		log.Info("apagando la api")
		ctxApagado, cancelar := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancelar()
		return srv.Shutdown(ctxApagado)
	}
}
