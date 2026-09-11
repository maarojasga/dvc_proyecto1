// Comando de migraciones.
//
// La API las aplica al arrancar, que es cómodo en desarrollo, pero no sirve
// para dos cosas que sí hacen falta:
//
//   - CI tiene que poder ejercerlas como un paso propio, para que una
//     migración que no aplica falle con ese nombre y no escondida dentro de un
//     test.
//   - Restaurar una copia de seguridad implica aplicar lo que le falte a la
//     copia, sin levantar la API.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/DES-SOLUCIONES-CLOUD/proyecto-1/backend/internal/config"
	"github.com/DES-SOLUCIONES-CLOUD/proyecto-1/backend/internal/platform/postgres"
	"github.com/DES-SOLUCIONES-CLOUD/proyecto-1/backend/migrations"
)

func main() {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, `Uso: migrate <orden>

Órdenes:
  up       aplica las migraciones pendientes
  status   lista lo aplicado y lo pendiente, sin cambiar nada

La conexión sale de DATABASE_URL.

No hay orden "down" a propósito: deshacer una migración en producción borra
datos, y la forma segura de volver atrás es restaurar una copia y aplicar hacia
adelante. Los archivos .down.sql existen para desarrollo y para revisar qué
haría cada paso al revés.
`)
	}
	flag.Parse()

	orden := flag.Arg(0)
	if orden == "" {
		flag.Usage()
		os.Exit(2)
	}

	cfg := config.Load()
	// Un plazo acotado: si la base no responde, es mejor fallar que dejar CI o
	// un despliegue colgados sin explicación.
	ctx, cancelar := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancelar()

	pool, err := postgres.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("migrate: no se pudo conectar: %v", err)
	}
	defer pool.Close()

	switch orden {
	case "up":
		if err := postgres.Migrate(ctx, pool, migrations.FS); err != nil {
			log.Fatalf("migrate: %v", err)
		}
		log.Print("migrate: al día")
	case "status":
		aplicadas, pendientes, err := postgres.EstadoDeMigraciones(ctx, pool, migrations.FS)
		if err != nil {
			log.Fatalf("migrate: %v", err)
		}
		for _, v := range aplicadas {
			fmt.Printf("aplicada   %s\n", v)
		}
		for _, v := range pendientes {
			fmt.Printf("pendiente  %s\n", v)
		}
		// Salida distinta de cero si falta algo: así un despliegue puede
		// comprobar el estado sin interpretar texto.
		if len(pendientes) > 0 {
			os.Exit(1)
		}
	default:
		fmt.Fprintf(os.Stderr, "migrate: orden desconocida %q\n\n", orden)
		flag.Usage()
		os.Exit(2)
	}
}
