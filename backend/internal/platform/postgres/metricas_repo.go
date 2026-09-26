package postgres

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/DES-SOLUCIONES-CLOUD/proyecto-1/backend/internal/domain/quiz"
)

// MetricasRepo reúne las consultas agregadas del panel administrativo
// (alcance opcional 5.2).
//
// Va en su propio repositorio y no repartido entre los demás porque son
// consultas de lectura que cruzan varias tablas y no pertenecen al agregado de
// ninguna: mezclarlas con las de escritura confundiría dos cosas que cambian
// por motivos distintos.
type MetricasRepo struct{ pool *pgxpool.Pool }

func NewMetricasRepo(pool *pgxpool.Pool) *MetricasRepo { return &MetricasRepo{pool: pool} }

// Metricas es la foto de la plataforma.
type Metricas struct {
	Usuarios      Conteo `json:"usuarios"`
	Cursos        Conteo `json:"cursos"`
	Inscripciones Conteo `json:"inscripciones"`
	Insignias     Conteo `json:"insignias"`
	Multimedia    Conteo `json:"multimedia"`
	Evaluaciones  Conteo `json:"evaluaciones"`
}

// Conteo es un total con su desglose. El desglose se devuelve como mapa y no
// como campos fijos para que añadir un estado en la base no obligue a tocar
// tres capas.
type Conteo struct {
	Total    int            `json:"total"`
	Desglose map[string]int `json:"desglose"`
}

// Resumen calcula todas las métricas del panel.
//
// Cada consulta agrupa por su columna de estado; el total se suma en Go en vez
// de pedir un COUNT aparte, que sería una ida y vuelta más para un número que
// ya está en la respuesta.
func (r *MetricasRepo) Resumen(ctx context.Context) (*Metricas, error) {
	m := &Metricas{}
	consultas := []struct {
		destino *Conteo
		sql     string
	}{
		{&m.Usuarios, `SELECT role || '/' || status, count(*) FROM users GROUP BY 1`},
		{&m.Cursos, `SELECT status, count(*) FROM course_versions GROUP BY 1`},
		{&m.Inscripciones, `SELECT status, count(*) FROM enrollments GROUP BY 1`},
		{&m.Insignias, `
			SELECT CASE WHEN revoked_at IS NULL THEN 'vigentes' ELSE 'revocadas' END, count(*)
			  FROM badges GROUP BY 1`},
		{&m.Multimedia, `SELECT status, count(*) FROM media_assets GROUP BY 1`},
		{&m.Evaluaciones, `SELECT status, count(*) FROM quiz_attempts GROUP BY 1`},
	}
	for _, c := range consultas {
		conteo, err := r.agrupar(ctx, c.sql)
		if err != nil {
			return nil, err
		}
		*c.destino = conteo
	}
	return m, nil
}

func (r *MetricasRepo) agrupar(ctx context.Context, sql string) (Conteo, error) {
	out := Conteo{Desglose: map[string]int{}}
	filas, err := r.pool.Query(ctx, sql)
	if err != nil {
		return out, err
	}
	defer filas.Close()
	for filas.Next() {
		var clave string
		var n int
		if err := filas.Scan(&clave, &n); err != nil {
			return out, err
		}
		out.Desglose[clave] = n
		out.Total += n
	}
	return out, filas.Err()
}

// IntentosDeQuiz devuelve los intentos ya calificados de una evaluación, con
// sus respuestas, para agregarlos.
//
// Devuelve también cuántas inscripciones distintas la presentaron, porque
// contar intentos no es contar personas.
//
// A escala de decenas de miles de intentos esto debería precalcularse en una
// vista materializada; con el volumen del MVP, y siendo una consulta que solo
// hace el profesor dueño o la administración, agregar al vuelo es más simple y
// no queda desactualizado.
func (r *MetricasRepo) IntentosDeQuiz(ctx context.Context, quizID uuid.UUID) ([]quiz.IntentoCalificado, int, error) {
	filas, err := r.pool.Query(ctx, `
		SELECT a.id, a.snapshot, coalesce(a.score, 0), coalesce(a.passed, false)
		  FROM quiz_attempts a
		 WHERE a.quiz_id = $1 AND a.status = 'submitted'
		 ORDER BY a.started_at`, quizID)
	if err != nil {
		return nil, 0, err
	}

	type intentoCrudo struct {
		id       uuid.UUID
		snapshot quiz.Snapshot
		score    float64
		passed   bool
	}
	var crudos []intentoCrudo
	for filas.Next() {
		var c intentoCrudo
		var snapshotJSON []byte
		if err := filas.Scan(&c.id, &snapshotJSON, &c.score, &c.passed); err != nil {
			filas.Close()
			return nil, 0, err
		}
		if err := json.Unmarshal(snapshotJSON, &c.snapshot); err != nil {
			filas.Close()
			return nil, 0, err
		}
		crudos = append(crudos, c)
	}
	filas.Close()
	if err := filas.Err(); err != nil {
		return nil, 0, err
	}

	intentos := make([]quiz.IntentoCalificado, 0, len(crudos))
	for _, c := range crudos {
		respuestas, err := r.respuestasDe(ctx, c.id)
		if err != nil {
			return nil, 0, err
		}
		intentos = append(intentos, quiz.IntentoCalificado{
			Snapshot: c.snapshot, Respuestas: respuestas, Score: c.score, Passed: c.passed,
		})
	}

	var estudiantes int
	if err := r.pool.QueryRow(ctx, `
		SELECT count(DISTINCT enrollment_id) FROM quiz_attempts
		 WHERE quiz_id = $1 AND status = 'submitted'`, quizID).Scan(&estudiantes); err != nil {
		return nil, 0, err
	}
	return intentos, estudiantes, nil
}

func (r *MetricasRepo) respuestasDe(ctx context.Context, attemptID uuid.UUID) (map[uuid.UUID][]uuid.UUID, error) {
	filas, err := r.pool.Query(ctx,
		`SELECT question_stable_id, selected_option_stable_ids FROM quiz_answers WHERE attempt_id = $1`, attemptID)
	if err != nil {
		return nil, err
	}
	defer filas.Close()

	out := map[uuid.UUID][]uuid.UUID{}
	for filas.Next() {
		var pregunta uuid.UUID
		var crudo []byte
		if err := filas.Scan(&pregunta, &crudo); err != nil {
			return nil, err
		}
		var seleccion []uuid.UUID
		if err := json.Unmarshal(crudo, &seleccion); err != nil {
			return nil, err
		}
		out[pregunta] = seleccion
	}
	return out, filas.Err()
}
