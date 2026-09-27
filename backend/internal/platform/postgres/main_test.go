package postgres_test

import (
	"os"
	"testing"

	"github.com/DES-SOLUCIONES-CLOUD/proyecto-1/backend/internal/platform/postgres/pgtest"
)

// Este paquete y el de httpserver corren contra la misma base y empiezan
// vaciando las mismas tablas, así que no pueden solaparse.
func TestMain(m *testing.M) {
	os.Exit(pgtest.ConBaseExclusiva(m))
}
