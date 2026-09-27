// Command migrate aplica las migraciones y termina.
//
// La API ya migra al arrancar, así que este comando no es la única vía: es la
// vía observable. La sección 10.1 exige que el pipeline de CI "complete
// migraciones" antes de la demostración, y comprobar eso arrancando la API
// obliga a distinguir un fallo de migración de un fallo de arranque leyendo
// registros. Aquí el código de salida lo dice.
//
// También sirve en un despliegue donde las migraciones se aplican una vez, en
// un paso previo, y no en cada réplica que arranca: con varias instancias
// migrando a la vez se depende de que el candado interno las serialice, y es
// más sencillo no ponerse en esa situación.
package main

import (
	"context"
	"log"
	"time"

	"github.com/DES-SOLUCIONES-CLOUD/proyecto-1/backend/internal/config"
	"github.com/DES-SOLUCIONES-CLOUD/proyecto-1/backend/internal/platform/postgres"

	"github.com/DES-SOLUCIONES-CLOUD/proyecto-1/backend/migrations"
)

func main() {
	cfg := config.Load()

	// Con límite: una migración que se queda esperando un candado es un
	// incidente, y colgarse para siempre lo convierte en un misterio.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	pool, err := postgres.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("migrate: postgres: %v", err)
	}
	defer pool.Close()

	if err := postgres.Migrate(ctx, pool, migrations.FS); err != nil {
		log.Fatalf("migrate: %v", err)
	}
	log.Println("migrate: migraciones aplicadas")
}
