package postgres_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/DES-SOLUCIONES-CLOUD/proyecto-1/backend/internal/platform/postgres"
	"github.com/DES-SOLUCIONES-CLOUD/proyecto-1/backend/migrations"
)

// baseDePruebas abre la base y deja el esquema aplicado.
func baseDePruebas(t *testing.T) *pgxpool.Pool {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("falta TEST_DATABASE_URL: se omite la prueba de integración")
	}
	ctx := context.Background()
	pool, err := postgres.Connect(ctx, url)
	if err != nil {
		t.Fatalf("PostgreSQL: %v", err)
	}
	t.Cleanup(pool.Close)
	if err := postgres.Migrate(ctx, pool, migrations.FS); err != nil {
		t.Fatalf("migraciones: %v", err)
	}
	if _, err := pool.Exec(ctx, `TRUNCATE audit_logs, users, courses CASCADE`); err != nil {
		t.Fatalf("limpieza: %v", err)
	}
	return pool
}

// activoDePrueba crea la cadena mínima (usuario, curso, versión, módulo,
// unidad, recurso) y devuelve el activo multimedia asociado.
func activoDePrueba(t *testing.T, pool *pgxpool.Pool, estado string) uuid.UUID {
	t.Helper()
	ctx := context.Background()

	profesorID, cursoID, versionID := uuid.New(), uuid.New(), uuid.New()
	moduloID, unidadID, recursoID, activoID := uuid.New(), uuid.New(), uuid.New(), uuid.New()

	sentencias := []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO users (id, email, password_hash, full_name, role, status)
		  VALUES ($1, $2, 'x', 'Profesor', 'teacher', 'active')`, []any{profesorID, profesorID.String() + "@example.com"}},
		{`INSERT INTO courses (id, teacher_id, slug) VALUES ($1, $2, $3)`,
			[]any{cursoID, profesorID, "curso-" + cursoID.String()[:8]}},
		{`INSERT INTO course_versions (id, course_id, version_number, status, title)
		  VALUES ($1, $2, 1, 'draft', 'Curso')`, []any{versionID, cursoID}},
		{`INSERT INTO modules (id, course_version_id, title, position) VALUES ($1, $2, 'M', 1)`,
			[]any{moduloID, versionID}},
		{`INSERT INTO units (id, module_id, title, position) VALUES ($1, $2, 'U', 1)`,
			[]any{unidadID, moduloID}},
		{`INSERT INTO resources (id, unit_id, type, title, position, visible)
		  VALUES ($1, $2, 'video', 'V', 1, true)`, []any{recursoID, unidadID}},
		{`INSERT INTO media_assets (id, resource_id, original_object_key, status)
		  VALUES ($1, $2, 'originales/v.mp4', $3)`, []any{activoID, recursoID, estado}},
	}
	for _, s := range sentencias {
		if _, err := pool.Exec(ctx, s.sql, s.args...); err != nil {
			t.Fatalf("preparando datos (%s): %v", s.sql[:40], err)
		}
	}
	return activoID
}

func TestReclamarSoloLoLograUnWorker(t *testing.T) {
	pool := baseDePruebas(t)
	repo := postgres.NewMediaRepo(pool)
	activoID := activoDePrueba(t, pool, "uploaded")
	ctx, ahora := context.Background(), time.Now().UTC()

	primero, err := repo.Reclamar(ctx, activoID, ahora)
	if err != nil {
		t.Fatalf("Reclamar: %v", err)
	}
	if !primero {
		t.Fatal("el primer intento debería tomar el trabajo")
	}

	// Segunda entrega del mismo trabajo mientras el primero sigue corriendo.
	// Sin esto, ambos transcodificarían y subirían salidas duplicadas.
	segundo, err := repo.Reclamar(ctx, activoID, ahora.Add(time.Minute))
	if err != nil {
		t.Fatalf("Reclamar: %v", err)
	}
	if segundo {
		t.Error("una entrega duplicada no debería tomar el trabajo en curso")
	}
}

func TestUnActivoListoNoSeVuelveAProcesar(t *testing.T) {
	pool := baseDePruebas(t)
	repo := postgres.NewMediaRepo(pool)
	activoID := activoDePrueba(t, pool, "uploaded")
	ctx := context.Background()

	if _, err := repo.Reclamar(ctx, activoID, time.Now().UTC()); err != nil {
		t.Fatalf("Reclamar: %v", err)
	}
	if err := repo.MarkReady(ctx, activoID, "hls/x/master.m3u8"); err != nil {
		t.Fatalf("MarkReady: %v", err)
	}

	reclamado, err := repo.Reclamar(ctx, activoID, time.Now().UTC())
	if err != nil {
		t.Fatalf("Reclamar: %v", err)
	}
	if reclamado {
		t.Error("un activo ya listo no debería reprocesarse")
	}

	// Y la salida anterior sigue en pie.
	activo, err := repo.GetByID(ctx, activoID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if activo.Status != "ready" || activo.HLSMasterKey != "hls/x/master.m3u8" {
		t.Errorf("el activo debería seguir listo: %+v", activo)
	}
}

func TestUnTrabajoAbandonadoSeRecuperaAlVencerElArrendamiento(t *testing.T) {
	pool := baseDePruebas(t)
	repo := postgres.NewMediaRepo(pool)
	activoID := activoDePrueba(t, pool, "uploaded")
	ctx := context.Background()

	inicio := time.Now().UTC()
	if _, err := repo.Reclamar(ctx, activoID, inicio); err != nil {
		t.Fatalf("Reclamar: %v", err)
	}

	// El worker que lo tomó muere sin marcar nada. Sin arrendamiento, el
	// recurso quedaría en 'processing' para siempre.
	reclamado, err := repo.Reclamar(ctx, activoID, inicio.Add(postgres.LeaseProcesamiento+time.Minute))
	if err != nil {
		t.Fatalf("Reclamar: %v", err)
	}
	if !reclamado {
		t.Error("vencido el arrendamiento, otro worker debería poder recogerlo")
	}
}

func TestUnFalloLiberaElArrendamientoParaElReintento(t *testing.T) {
	pool := baseDePruebas(t)
	repo := postgres.NewMediaRepo(pool)
	activoID := activoDePrueba(t, pool, "uploaded")
	ctx, ahora := context.Background(), time.Now().UTC()

	if _, err := repo.Reclamar(ctx, activoID, ahora); err != nil {
		t.Fatalf("Reclamar: %v", err)
	}
	if err := repo.MarkFailed(ctx, activoID, "ffmpeg reventó"); err != nil {
		t.Fatalf("MarkFailed: %v", err)
	}

	// El reintento de asynq llega enseguida y debe poder tomarlo, sin esperar
	// a que venza el arrendamiento del intento que ya falló.
	reclamado, err := repo.Reclamar(ctx, activoID, ahora.Add(time.Second))
	if err != nil {
		t.Fatalf("Reclamar: %v", err)
	}
	if !reclamado {
		t.Error("tras un fallo, el reintento debería poder tomar el trabajo")
	}
}

func TestElOriginalNuncaSeToca(t *testing.T) {
	pool := baseDePruebas(t)
	repo := postgres.NewMediaRepo(pool)
	activoID := activoDePrueba(t, pool, "uploaded")
	ctx := context.Background()

	antes, err := repo.GetByID(ctx, activoID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if _, err := repo.Reclamar(ctx, activoID, time.Now().UTC()); err != nil {
		t.Fatalf("Reclamar: %v", err)
	}
	if err := repo.MarkReady(ctx, activoID, "hls/x/master.m3u8"); err != nil {
		t.Fatalf("MarkReady: %v", err)
	}

	despues, err := repo.GetByID(ctx, activoID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	// El HLS es una salida derivada: se guarda aparte y el original queda
	// intacto, que es lo que exige la conservación del archivo fuente.
	if despues.OriginalObjectKey != antes.OriginalObjectKey {
		t.Errorf("la clave del original cambió: %q -> %q", antes.OriginalObjectKey, despues.OriginalObjectKey)
	}
	if despues.HLSMasterKey == despues.OriginalObjectKey {
		t.Error("el HLS no debería sobrescribir el original")
	}
}
