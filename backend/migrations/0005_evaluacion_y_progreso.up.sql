-- Evaluacion academica, progreso verificado e insignias.
--
-- Las tablas ya existen desde 0001_init; aqui se agregan las restricciones e
-- indices que exigen las condiciones verificables del proyecto y que no se
-- pueden garantizar solo desde el codigo.

-- Integridad del quiz: el envio definitivo es idempotente. Reintentar con la
-- misma Idempotency-Key no puede producir una segunda calificacion, y eso se
-- garantiza en la base y no solo en memoria, porque la API corre en varias
-- instancias que no comparten estado.
CREATE UNIQUE INDEX idx_quiz_attempts_submit_idempotency
    ON quiz_attempts (enrollment_id, submit_idempotency_key)
    WHERE submit_idempotency_key IS NOT NULL AND submit_idempotency_key <> '';

-- Un estudiante no puede tener dos intentos abiertos sobre el mismo quiz: el
-- que reanuda debe encontrar el suyo, no crear otro.
CREATE UNIQUE INDEX idx_quiz_attempts_uno_en_curso
    ON quiz_attempts (quiz_id, enrollment_id)
    WHERE status = 'in_progress';

CREATE INDEX idx_quiz_attempts_inscripcion ON quiz_attempts (enrollment_id, quiz_id);
CREATE INDEX idx_quiz_answers_intento ON quiz_answers (attempt_id);
CREATE INDEX idx_quiz_questions_quiz ON quiz_questions (quiz_id, position);
CREATE INDEX idx_quiz_options_pregunta ON quiz_options (question_id, position);

-- El calculo de permanencia consulta el ultimo evento de un recurso, asi que
-- el indice se ordena por fecha descendente.
CREATE INDEX idx_progress_events_recurso
    ON progress_events (enrollment_id, resource_stable_id, occurred_at DESC);

COMMENT ON COLUMN quiz_attempts.snapshot IS
    'Copia congelada de preguntas y opciones mostradas, con la clave correcta. '
    'Nunca se serializa completo hacia el cliente: hace la calificacion '
    'reproducible aunque el autor edite el quiz despues.';

COMMENT ON COLUMN badges.verification_code IS
    'Codigo publico aleatorio de la URL de verificacion. No deriva del correo '
    'ni de ningun dato personal del estudiante.';
