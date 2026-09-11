// Sembrador del juego de datos para la prueba de carga.
//
// El objetivo del enunciado son 50.000 usuarios registrados, y sembrarlos por
// la API no es viable: el hash de contraseña es bcrypt con coste 12, unos 250
// ms por cuenta a propósito, así que 50.000 altas serían más de tres horas de
// espera antes de empezar a medir.
//
// Por eso este comando escribe en la base directamente y con un único hash
// precalculado, compartido por todas las cuentas sembradas. Es aceptable solo
// porque es un juego de datos de prueba, y por eso el comando se niega a
// ejecutarse con APP_ENV=production: una base real con 50.000 cuentas que
// comparten una contraseña conocida es una brecha, no un juego de datos.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/DES-SOLUCIONES-CLOUD/proyecto-1/backend/internal/config"
	"github.com/DES-SOLUCIONES-CLOUD/proyecto-1/backend/internal/platform/postgres"
	"github.com/DES-SOLUCIONES-CLOUD/proyecto-1/backend/internal/platform/security"
)

// ClaveDeCarga es la contraseña de todas las cuentas sembradas. Va a la vista
// porque el script de k6 la necesita y porque no protege nada: son cuentas de
// un entorno de prueba que se destruye después.
const ClaveDeCarga = "CargaDePrueba123!"

// dominioDeCarga marca las cuentas sembradas, para poder borrarlas sin tocar
// nada más de la base.
const dominioDeCarga = "carga.local"

func main() {
	var (
		estudiantes   = flag.Int("estudiantes", 50_000, "cuántas cuentas de estudiante sembrar")
		cursos        = flag.Int("cursos", 200, "cuántos cursos publicados sembrar")
		inscripciones = flag.Int("inscripciones", 5, "en cuántos cursos inscribir a cada estudiante")
		limpiar       = flag.Bool("limpiar", false, "borrar lo sembrado y salir")
		lote          = flag.Int("lote", 1000, "filas por transacción")
	)
	flag.Parse()

	cfg := config.Load()
	if cfg.Env == "production" {
		log.Fatal("sembrar-carga: no se ejecuta con APP_ENV=production; 50.000 cuentas con una contraseña conocida es una brecha")
	}

	ctx, cancelar := context.WithTimeout(context.Background(), 2*time.Hour)
	defer cancelar()

	pool, err := postgres.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("sembrar-carga: no se pudo conectar: %v", err)
	}
	defer pool.Close()

	if *limpiar {
		if err := limpiarSembrado(ctx, pool); err != nil {
			log.Fatalf("sembrar-carga: %v", err)
		}
		log.Print("sembrar-carga: juego de datos borrado")
		return
	}

	// Un solo hash para todas: calcularlo una vez por cuenta es exactamente lo
	// que hace inviable sembrar por la API.
	hash, err := security.HashPassword(ClaveDeCarga)
	if err != nil {
		log.Fatalf("sembrar-carga: no se pudo calcular el hash: %v", err)
	}

	inicio := time.Now()

	profesorID, err := sembrarProfesor(ctx, pool, hash)
	if err != nil {
		log.Fatalf("sembrar-carga: profesor: %v", err)
	}

	idsCursos, err := sembrarCursos(ctx, pool, profesorID, *cursos)
	if err != nil {
		log.Fatalf("sembrar-carga: cursos: %v", err)
	}
	log.Printf("sembrar-carga: %d cursos publicados", len(idsCursos))

	sembrados, err := sembrarEstudiantes(ctx, pool, hash, *estudiantes, *lote)
	if err != nil {
		log.Fatalf("sembrar-carga: estudiantes: %v", err)
	}
	log.Printf("sembrar-carga: %d estudiantes", sembrados)

	inscritos, err := sembrarInscripciones(ctx, pool, sembrados, idsCursos, *inscripciones, *lote)
	if err != nil {
		log.Fatalf("sembrar-carga: inscripciones: %v", err)
	}
	log.Printf("sembrar-carga: %d inscripciones", inscritos)

	// ANALYZE explícito: sin estadísticas al día el planificador elige planes
	// malos sobre las tablas recién llenadas, y la primera medición saldría
	// peor de lo que es el sistema.
	if _, err := pool.Exec(ctx, "ANALYZE"); err != nil {
		log.Printf("sembrar-carga: ANALYZE falló: %v", err)
	}

	log.Printf("sembrar-carga: listo en %s", time.Since(inicio).Round(time.Second))
	// Lo que el script de k6 necesita, en formato de entorno para poder
	// evaluarlo directamente.
	fmt.Printf("CARGA_CORREO_PATRON=estudiante%%d@%s\nCARGA_CLAVE=%s\nCARGA_ESTUDIANTES=%d\n",
		dominioDeCarga, ClaveDeCarga, sembrados)
}

func sembrarProfesor(ctx context.Context, pool *pgxpool.Pool, hash string) (uuid.UUID, error) {
	var id uuid.UUID
	err := pool.QueryRow(ctx, `
		INSERT INTO users (email, password_hash, full_name, role, status, email_verified_at)
		VALUES ($1, $2, 'Profesor de carga', 'teacher', 'active', now())
		ON CONFLICT (email) DO UPDATE SET updated_at = now()
		RETURNING id`, "profesor@"+dominioDeCarga, hash).Scan(&id)
	return id, err
}

// sembrarEstudiantes inserta las cuentas por lotes y devuelve cuántas hay.
//
// El correo se deriva del índice (estudiante0@…, estudiante1@…) para que el
// script de k6 pueda elegir una cuenta por número de usuario virtual sin
// necesitar una lista: con 50.000 cuentas, pasar la lista al generador de
// carga costaría más que la propia prueba.
func sembrarEstudiantes(ctx context.Context, pool *pgxpool.Pool, hash string, cuantos, lote int) (int, error) {
	for desde := 0; desde < cuantos; desde += lote {
		hasta := desde + lote
		if hasta > cuantos {
			hasta = cuantos
		}
		// generate_series hace el trabajo en la base: mandar 50.000 INSERT
		// desde aquí serían 50.000 idas y vueltas.
		_, err := pool.Exec(ctx, `
			INSERT INTO users (email, password_hash, full_name, role, status, email_verified_at)
			SELECT format('estudiante%s@`+dominioDeCarga+`', i), $3,
			       format('Estudiante %s', i), 'student', 'active', now()
			FROM generate_series($1, $2 - 1) AS i
			ON CONFLICT (email) DO NOTHING`, desde, hasta, hash)
		if err != nil {
			return 0, fmt.Errorf("estudiantes [%d,%d): %w", desde, hasta, err)
		}
		if (desde/lote)%10 == 0 {
			log.Printf("sembrar-carga: %d/%d estudiantes", hasta, cuantos)
		}
	}
	var n int
	err := pool.QueryRow(ctx,
		`SELECT count(*) FROM users WHERE email LIKE 'estudiante%@`+dominioDeCarga+`'`).Scan(&n)
	return n, err
}

// sembrarCursos crea cursos publicados con estructura mínima válida.
//
// Cada curso lleva un módulo, una unidad y un recurso de texto visible y
// obligatorio: es lo mínimo que la validación de publicación acepta, y basta
// para que el catálogo y el consumo de contenido tengan algo real que servir.
func sembrarCursos(ctx context.Context, pool *pgxpool.Pool, profesorID uuid.UUID, cuantos int) ([]uuid.UUID, error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	niveles := []string{"basico", "intermedio", "avanzado"}
	categorias := []string{"Cloud", "Datos", "Redes", "Seguridad", "Desarrollo"}

	var ids []uuid.UUID
	for i := 0; i < cuantos; i++ {
		var cursoID, versionID, moduloID, unidadID uuid.UUID
		slug := fmt.Sprintf("carga-curso-%d", i)

		err := tx.QueryRow(ctx, `
			INSERT INTO courses (teacher_id, slug) VALUES ($1, $2)
			ON CONFLICT (slug) DO UPDATE SET updated_at = now()
			RETURNING id`, profesorID, slug).Scan(&cursoID)
		if err != nil {
			return nil, fmt.Errorf("curso %d: %w", i, err)
		}

		// published_at escalonado: si todos los cursos se publicaran en el
		// mismo instante, el desempate por id sería lo único que ordenaría el
		// catálogo y la paginación por cursor no se ejercitaría de verdad.
		publicado := time.Now().UTC().Add(-time.Duration(cuantos-i) * time.Minute)
		err = tx.QueryRow(ctx, `
			INSERT INTO course_versions (course_id, version_number, status, title, summary,
			                             description_md, category, level, language,
			                             approval_min_score, approval_required_resources_pct, published_at)
			VALUES ($1, 1, 'published', $2, $3, $4, $5, $6, 'es', 60, 100, $7)
			RETURNING id`,
			cursoID,
			fmt.Sprintf("Curso de carga %d", i),
			fmt.Sprintf("Resumen del curso %d, sembrado para la prueba de carga.", i),
			fmt.Sprintf("# Curso %d\n\nContenido de prueba.", i),
			categorias[i%len(categorias)], niveles[i%len(niveles)], publicado,
		).Scan(&versionID)
		if err != nil {
			return nil, fmt.Errorf("versión %d: %w", i, err)
		}

		if _, err := tx.Exec(ctx,
			`UPDATE courses SET current_published_version_id = $2 WHERE id = $1`, cursoID, versionID); err != nil {
			return nil, fmt.Errorf("vigente %d: %w", i, err)
		}

		if err := tx.QueryRow(ctx, `
			INSERT INTO modules (course_version_id, title, position) VALUES ($1, 'Módulo 1', 1)
			RETURNING id`, versionID).Scan(&moduloID); err != nil {
			return nil, fmt.Errorf("módulo %d: %w", i, err)
		}
		if err := tx.QueryRow(ctx, `
			INSERT INTO units (module_id, title, position) VALUES ($1, 'Unidad 1', 1)
			RETURNING id`, moduloID).Scan(&unidadID); err != nil {
			return nil, fmt.Errorf("unidad %d: %w", i, err)
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO resources (unit_id, type, title, position, visible, required, downloadable,
			                       processing_status, text_content_md)
			VALUES ($1, 'text', 'Lectura 1', 1, true, true, false, 'none', $2)`,
			unidadID, "# Lectura\n\nMaterial de prueba para la carga."); err != nil {
			return nil, fmt.Errorf("recurso %d: %w", i, err)
		}

		ids = append(ids, cursoID)
	}
	return ids, tx.Commit(ctx)
}

// sembrarInscripciones inscribe a cada estudiante en unos cuantos cursos.
//
// El reparto es determinista (estudiante i en los cursos i, i+1, …) y no
// aleatorio: así una ejecución se puede repetir y comparar con la anterior, que
// es lo que hace útil una prueba de carga.
func sembrarInscripciones(ctx context.Context, pool *pgxpool.Pool, estudiantes int, cursos []uuid.UUID, porEstudiante, lote int) (int, error) {
	if len(cursos) == 0 || porEstudiante <= 0 {
		return 0, nil
	}
	for desde := 0; desde < estudiantes; desde += lote {
		hasta := desde + lote
		if hasta > estudiantes {
			hasta = estudiantes
		}
		_, err := pool.Exec(ctx, `
			WITH numerados AS (
				SELECT u.id,
				       (regexp_replace(u.email, '\D', '', 'g'))::bigint AS n
				FROM users u
				WHERE u.email LIKE 'estudiante%@`+dominioDeCarga+`'
				  AND (regexp_replace(u.email, '\D', '', 'g'))::bigint >= $1
				  AND (regexp_replace(u.email, '\D', '', 'g'))::bigint < $2
			),
			cursos_ordenados AS (
				SELECT c.id, c.current_published_version_id,
				       row_number() OVER (ORDER BY c.slug) - 1 AS idx,
				       count(*) OVER () AS total
				FROM courses c
				WHERE c.slug LIKE 'carga-curso-%'
				  AND c.current_published_version_id IS NOT NULL
			)
			INSERT INTO enrollments (student_id, course_id, course_version_id, status, enrolled_at)
			SELECT e.id, c.id, c.current_published_version_id, 'active', now()
			FROM numerados e
			CROSS JOIN generate_series(0, $3 - 1) AS k
			JOIN cursos_ordenados c ON c.idx = (e.n + k) % c.total
			ON CONFLICT (student_id, course_id) DO NOTHING`, desde, hasta, porEstudiante)
		if err != nil {
			return 0, fmt.Errorf("inscripciones [%d,%d): %w", desde, hasta, err)
		}
	}
	var n int
	err := pool.QueryRow(ctx, `
		SELECT count(*) FROM enrollments e
		JOIN users u ON u.id = e.student_id
		WHERE u.email LIKE 'estudiante%@`+dominioDeCarga+`'`).Scan(&n)
	return n, err
}

// limpiarSembrado borra el juego de datos.
//
// Basta borrar las cuentas y los cursos sembrados: el resto cae por las claves
// ajenas en cascada. Se identifican por el dominio y el prefijo del slug, así
// que no puede llevarse por delante datos que no sembró este comando.
func limpiarSembrado(ctx context.Context, pool *pgxpool.Pool) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	for _, sentencia := range []string{
		`DELETE FROM courses WHERE slug LIKE 'carga-curso-%'`,
		`DELETE FROM users WHERE email LIKE '%@` + dominioDeCarga + `'`,
	} {
		if _, err := tx.Exec(ctx, sentencia); err != nil {
			return fmt.Errorf("limpiando: %w", err)
		}
	}
	return tx.Commit(ctx)
}
