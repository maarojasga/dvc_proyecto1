// Package pgtest coordina los paquetes de prueba que comparten la base de
// datos de integración.
//
// `go test ./...` ejecuta los paquetes en paralelo, y hay más de uno que corre
// contra PostgreSQL y empieza vaciando las mismas tablas. Sin coordinación, el
// TRUNCATE de un paquete borra los datos que otro acaba de sembrar y la suite
// falla de forma intermitente, con pruebas que pasan cuando se ejecutan
// aisladas. Un fallo así no es un detalle de comodidad: la especificación
// acepta un requisito solo si sus pruebas son reproducibles, y el pipeline de
// CI ejecuta la suite completa.
//
// La exclusión se apoya en un cerrojo consultivo de PostgreSQL, que es de
// sesión y lo libera la propia base si el proceso muere: no deja cerrojos
// huérfanos que obliguen a limpiar a mano.
package pgtest

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5"
)

// claveDelCerrojo identifica el recurso compartido —la base de integración—.
// El valor no significa nada; solo tiene que ser el mismo en todos los
// paquetes que la usan.
const claveDelCerrojo = 728_301_945

// ConBaseExclusiva ejecuta la suite del paquete con acceso exclusivo a la base
// de integración.
//
// Se llama desde TestMain. Si no hay TEST_DATABASE_URL, las pruebas de
// integración se omiten solas y esto no hace nada.
func ConBaseExclusiva(m *testing.M) int {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		return m.Run()
	}

	ctx := context.Background()
	// Una conexión propia, fuera de cualquier pool: el cerrojo vive en la
	// sesión que lo toma, y en un pool la siguiente consulta podría salir por
	// otra conexión distinta.
	conn, err := pgx.Connect(ctx, url)
	if err != nil {
		// Sin base no hay nada que coordinar; las pruebas que la necesiten se
		// saltarán o fallarán por su cuenta, con su propio mensaje.
		return m.Run()
	}
	defer conn.Close(ctx)

	if _, err := conn.Exec(ctx, `SELECT pg_advisory_lock($1)`, claveDelCerrojo); err != nil {
		return m.Run()
	}
	defer func() { _, _ = conn.Exec(ctx, `SELECT pg_advisory_unlock($1)`, claveDelCerrojo) }()

	return m.Run()
}
