package postgres

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/DES-SOLUCIONES-CLOUD/proyecto-1/backend/internal/domain/quiz"
)

// QuizRepo persiste evaluaciones, intentos y respuestas.
//
// La clave correcta (quiz_options.is_correct) solo sale de aqui hacia el
// servicio, nunca hacia una respuesta HTTP: el dominio expone ToPublic() para
// la proyeccion que si puede viajar al cliente.
type QuizRepo struct{ pool *pgxpool.Pool }

func NewQuizRepo(pool *pgxpool.Pool) *QuizRepo { return &QuizRepo{pool: pool} }

// GetByResource carga el quiz completo asociado a un recurso, con sus
// preguntas y opciones ordenadas por posicion.
func (r *QuizRepo) GetByResource(ctx context.Context, resourceID uuid.UUID) (*quiz.Quiz, error) {
	var q quiz.Quiz
	var politica string
	err := r.pool.QueryRow(ctx, `
		SELECT id, resource_id, title, time_limit_seconds, max_attempts,
		       pass_score, feedback_policy, shuffle_questions
		  FROM quizzes WHERE resource_id = $1`, resourceID).
		Scan(&q.ID, &q.ResourceID, &q.Title, &q.TimeLimitSeconds, &q.MaxAttempts,
			&q.PassScore, &politica, &q.ShuffleQuestions)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	q.FeedbackPolicy = quiz.FeedbackPolicy(politica)

	preguntas, err := r.preguntas(ctx, q.ID)
	if err != nil {
		return nil, err
	}
	q.Questions = preguntas
	return &q, nil
}

// GuardarDefinicion crea o reemplaza el quiz de un recurso.
//
// Va en una transaccion y borra las preguntas anteriores porque un quiz a
// medio reemplazar dejaria intentos imposibles de calificar. Los intentos ya
// cerrados no se ven afectados: guardan su propio snapshot.
func (r *QuizRepo) GuardarDefinicion(ctx context.Context, q *quiz.Quiz) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	err = tx.QueryRow(ctx, `
		INSERT INTO quizzes (resource_id, title, time_limit_seconds, max_attempts,
		                     pass_score, feedback_policy, shuffle_questions, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, now())
		ON CONFLICT (resource_id) DO UPDATE
		   SET title = EXCLUDED.title,
		       time_limit_seconds = EXCLUDED.time_limit_seconds,
		       max_attempts = EXCLUDED.max_attempts,
		       pass_score = EXCLUDED.pass_score,
		       feedback_policy = EXCLUDED.feedback_policy,
		       shuffle_questions = EXCLUDED.shuffle_questions,
		       updated_at = now()
		RETURNING id`,
		q.ResourceID, q.Title, q.TimeLimitSeconds, q.MaxAttempts,
		q.PassScore, string(q.FeedbackPolicy), q.ShuffleQuestions).Scan(&q.ID)
	if err != nil {
		return err
	}

	if _, err := tx.Exec(ctx, `DELETE FROM quiz_questions WHERE quiz_id = $1`, q.ID); err != nil {
		return err
	}

	for i := range q.Questions {
		p := &q.Questions[i]
		if p.StableID == uuid.Nil {
			p.StableID = uuid.New()
		}
		err := tx.QueryRow(ctx, `
			INSERT INTO quiz_questions (quiz_id, stable_id, position, prompt_md, type, points)
			VALUES ($1, $2, $3, $4, $5, $6) RETURNING id`,
			q.ID, p.StableID, p.Position, p.PromptMD, string(p.Type), p.Points).Scan(&p.ID)
		if err != nil {
			return err
		}
		for j := range p.Options {
			o := &p.Options[j]
			if o.StableID == uuid.Nil {
				o.StableID = uuid.New()
			}
			err := tx.QueryRow(ctx, `
				INSERT INTO quiz_options (question_id, stable_id, position, text_md, is_correct)
				VALUES ($1, $2, $3, $4, $5) RETURNING id`,
				p.ID, o.StableID, o.Position, o.TextMD, o.IsCorrect).Scan(&o.ID)
			if err != nil {
				return err
			}
		}
	}
	return tx.Commit(ctx)
}

// PorID carga el quiz por su identificador, que es como lo referencia un
// intento ya iniciado.
func (r *QuizRepo) PorID(ctx context.Context, id uuid.UUID) (*quiz.Quiz, error) {
	var q quiz.Quiz
	var politica string
	err := r.pool.QueryRow(ctx, `
		SELECT id, resource_id, title, time_limit_seconds, max_attempts,
		       pass_score, feedback_policy, shuffle_questions
		  FROM quizzes WHERE id = $1`, id).
		Scan(&q.ID, &q.ResourceID, &q.Title, &q.TimeLimitSeconds, &q.MaxAttempts,
			&q.PassScore, &politica, &q.ShuffleQuestions)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	q.FeedbackPolicy = quiz.FeedbackPolicy(politica)

	preguntas, err := r.preguntas(ctx, q.ID)
	if err != nil {
		return nil, err
	}
	q.Questions = preguntas
	return &q, nil
}

func (r *QuizRepo) preguntas(ctx context.Context, quizID uuid.UUID) ([]quiz.Question, error) {
	filas, err := r.pool.Query(ctx, `
		SELECT id, stable_id, position, prompt_md, type, points
		  FROM quiz_questions WHERE quiz_id = $1 ORDER BY position`, quizID)
	if err != nil {
		return nil, err
	}
	defer filas.Close()

	var out []quiz.Question
	indice := map[uuid.UUID]int{}
	for filas.Next() {
		var p quiz.Question
		var tipo string
		if err := filas.Scan(&p.ID, &p.StableID, &p.Position, &p.PromptMD, &tipo, &p.Points); err != nil {
			return nil, err
		}
		p.Type = quiz.QuestionType(tipo)
		indice[p.ID] = len(out)
		out = append(out, p)
	}
	if err := filas.Err(); err != nil {
		return nil, err
	}
	if len(out) == 0 {
		return out, nil
	}

	// Las opciones se traen en una sola consulta y se reparten en memoria,
	// para no hacer una consulta por pregunta.
	opciones, err := r.pool.Query(ctx, `
		SELECT o.question_id, o.id, o.stable_id, o.position, o.text_md, o.is_correct
		  FROM quiz_options o
		  JOIN quiz_questions q ON q.id = o.question_id
		 WHERE q.quiz_id = $1
		 ORDER BY o.position`, quizID)
	if err != nil {
		return nil, err
	}
	defer opciones.Close()

	for opciones.Next() {
		var preguntaID uuid.UUID
		var o quiz.Option
		if err := opciones.Scan(&preguntaID, &o.ID, &o.StableID, &o.Position, &o.TextMD, &o.IsCorrect); err != nil {
			return nil, err
		}
		if i, ok := indice[preguntaID]; ok {
			out[i].Options = append(out[i].Options, o)
		}
	}
	return out, opciones.Err()
}

// ContarIntentos cuenta los intentos ya iniciados por una inscripcion sobre un
// quiz, incluidos los expirados: gastar el tiempo tambien consume el intento.
func (r *QuizRepo) ContarIntentos(ctx context.Context, quizID, enrollmentID uuid.UUID) (int, error) {
	var n int
	err := r.pool.QueryRow(ctx, `
		SELECT count(*) FROM quiz_attempts
		 WHERE quiz_id = $1 AND enrollment_id = $2`, quizID, enrollmentID).Scan(&n)
	return n, err
}

// IntentoEnCurso devuelve el intento abierto, si lo hay. Permite reanudar en
// vez de crear uno nuevo cuando el estudiante recarga la pagina.
func (r *QuizRepo) IntentoEnCurso(ctx context.Context, quizID, enrollmentID uuid.UUID) (*quiz.Attempt, error) {
	return r.leerIntento(ctx, `
		SELECT id, quiz_id, enrollment_id, attempt_number, status, snapshot,
		       started_at, submitted_at, expires_at, score, passed,
		       coalesce(submit_idempotency_key, '')
		  FROM quiz_attempts
		 WHERE quiz_id = $1 AND enrollment_id = $2 AND status = 'in_progress'`,
		quizID, enrollmentID)
}

func (r *QuizRepo) IntentoPorID(ctx context.Context, id uuid.UUID) (*quiz.Attempt, error) {
	return r.leerIntento(ctx, `
		SELECT id, quiz_id, enrollment_id, attempt_number, status, snapshot,
		       started_at, submitted_at, expires_at, score, passed,
		       coalesce(submit_idempotency_key, '')
		  FROM quiz_attempts WHERE id = $1`, id)
}

// IntentoPorClaveIdempotencia recupera el intento ya calificado con esa clave,
// para que un reintento devuelva la misma nota en lugar de recalcular.
func (r *QuizRepo) IntentoPorClaveIdempotencia(ctx context.Context, enrollmentID uuid.UUID, clave string) (*quiz.Attempt, error) {
	return r.leerIntento(ctx, `
		SELECT id, quiz_id, enrollment_id, attempt_number, status, snapshot,
		       started_at, submitted_at, expires_at, score, passed,
		       coalesce(submit_idempotency_key, '')
		  FROM quiz_attempts
		 WHERE enrollment_id = $1 AND submit_idempotency_key = $2`,
		enrollmentID, clave)
}

func (r *QuizRepo) leerIntento(ctx context.Context, sql string, args ...any) (*quiz.Attempt, error) {
	var a quiz.Attempt
	var estado string
	var crudo []byte
	err := r.pool.QueryRow(ctx, sql, args...).Scan(
		&a.ID, &a.QuizID, &a.EnrollmentID, &a.AttemptNumber, &estado, &crudo,
		&a.StartedAt, &a.SubmittedAt, &a.ExpiresAt, &a.Score, &a.Passed,
		&a.SubmitIdempotencyKey)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	a.Status = quiz.AttemptStatus(estado)
	if err := json.Unmarshal(crudo, &a.Snapshot); err != nil {
		return nil, err
	}
	return &a, nil
}

func (r *QuizRepo) CrearIntento(ctx context.Context, a *quiz.Attempt) error {
	crudo, err := json.Marshal(a.Snapshot)
	if err != nil {
		return err
	}
	_, err = r.pool.Exec(ctx, `
		INSERT INTO quiz_attempts
		    (id, quiz_id, enrollment_id, attempt_number, status, snapshot, started_at, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		a.ID, a.QuizID, a.EnrollmentID, a.AttemptNumber, string(a.Status), crudo,
		a.StartedAt, a.ExpiresAt)
	return err
}

// GuardarRespuesta persiste el guardado parcial de una pregunta. Es un upsert
// porque el estudiante puede cambiar de opinion antes de enviar.
func (r *QuizRepo) GuardarRespuesta(ctx context.Context, attemptID, preguntaStableID uuid.UUID, seleccion []uuid.UUID) error {
	crudo, err := json.Marshal(seleccion)
	if err != nil {
		return err
	}
	_, err = r.pool.Exec(ctx, `
		INSERT INTO quiz_answers (attempt_id, question_stable_id, selected_option_stable_ids, saved_at)
		VALUES ($1, $2, $3, now())
		ON CONFLICT (attempt_id, question_stable_id) DO UPDATE
		   SET selected_option_stable_ids = EXCLUDED.selected_option_stable_ids,
		       saved_at = now()`,
		attemptID, preguntaStableID, crudo)
	return err
}

// Respuestas devuelve lo guardado hasta ahora, que es lo que se califica al
// enviar: el cliente no vuelve a mandar todo el formulario.
func (r *QuizRepo) Respuestas(ctx context.Context, attemptID uuid.UUID) (map[uuid.UUID][]uuid.UUID, error) {
	filas, err := r.pool.Query(ctx, `
		SELECT question_stable_id, selected_option_stable_ids
		  FROM quiz_answers WHERE attempt_id = $1`, attemptID)
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

// CerrarIntento guarda la calificacion junto con la clave de idempotencia.
//
// La condicion sobre status hace que dos envios simultaneos no puedan calificar
// dos veces: el segundo actualiza cero filas y el servicio devuelve lo ya
// guardado.
func (r *QuizRepo) CerrarIntento(ctx context.Context, a *quiz.Attempt) (bool, error) {
	etiqueta, err := r.pool.Exec(ctx, `
		UPDATE quiz_attempts
		   SET status = $2, submitted_at = $3, score = $4, passed = $5,
		       submit_idempotency_key = nullif($6, '')
		 WHERE id = $1 AND status = 'in_progress'`,
		a.ID, string(a.Status), a.SubmittedAt, a.Score, a.Passed, a.SubmitIdempotencyKey)
	if err != nil {
		return false, err
	}
	return etiqueta.RowsAffected() == 1, nil
}

// ResultadoPorRecurso indica, para cada recurso de tipo quiz que la inscripcion
// haya intentado, si existe algun intento aprobado. Es lo que necesita el
// calculo de aprobacion del curso.
func (r *QuizRepo) ResultadoPorRecurso(ctx context.Context, enrollmentID uuid.UUID) (map[uuid.UUID]bool, error) {
	filas, err := r.pool.Query(ctx, `
		SELECT res.stable_id, bool_or(coalesce(a.passed, false))
		  FROM quiz_attempts a
		  JOIN quizzes q   ON q.id = a.quiz_id
		  JOIN resources res ON res.id = q.resource_id
		 WHERE a.enrollment_id = $1 AND a.status = 'submitted'
		 GROUP BY res.stable_id`, enrollmentID)
	if err != nil {
		return nil, err
	}
	defer filas.Close()

	out := map[uuid.UUID]bool{}
	for filas.Next() {
		var stableID uuid.UUID
		var aprobado bool
		if err := filas.Scan(&stableID, &aprobado); err != nil {
			return nil, err
		}
		out[stableID] = aprobado
	}
	return out, filas.Err()
}
