-- Esquema inicial de la Plataforma MOOC.
-- Convenciones: UUID como PK, timestamps en UTC, snake_case, enums como
-- CHECK constraints sobre TEXT para simplicidad de migraciones futuras.

CREATE EXTENSION IF NOT EXISTS pgcrypto;
CREATE EXTENSION IF NOT EXISTS citext;

-- ============================================================
-- Identidad y sesiones
-- ============================================================

CREATE TABLE users (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email             CITEXT NOT NULL UNIQUE,
    password_hash     TEXT NOT NULL,
    full_name         TEXT NOT NULL,
    role              TEXT NOT NULL CHECK (role IN ('admin', 'teacher', 'student')),
    status            TEXT NOT NULL DEFAULT 'pending_verification'
                          CHECK (status IN ('pending_verification', 'active', 'suspended')),
    email_verified_at TIMESTAMPTZ,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_users_role ON users (role);

CREATE TABLE email_verification_tokens (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    token_hash TEXT NOT NULL UNIQUE,
    expires_at TIMESTAMPTZ NOT NULL,
    used_at    TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE password_reset_tokens (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    token_hash TEXT NOT NULL UNIQUE,
    expires_at TIMESTAMPTZ NOT NULL,
    used_at    TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE sessions (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    token_hash TEXT NOT NULL UNIQUE,
    user_agent TEXT,
    ip_address TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at TIMESTAMPTZ NOT NULL,
    revoked_at TIMESTAMPTZ
);

CREATE INDEX idx_sessions_user_id ON sessions (user_id);

CREATE TABLE audit_logs (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    actor_user_id  UUID REFERENCES users (id) ON DELETE SET NULL,
    action         TEXT NOT NULL,
    entity_type    TEXT NOT NULL,
    entity_id      TEXT,
    metadata       JSONB NOT NULL DEFAULT '{}'::jsonb,
    ip_address     TEXT,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_audit_logs_actor ON audit_logs (actor_user_id);
CREATE INDEX idx_audit_logs_entity ON audit_logs (entity_type, entity_id);
CREATE INDEX idx_audit_logs_created_at ON audit_logs (created_at);

-- Idempotencia genérica para escrituras REST (header Idempotency-Key).
CREATE TABLE idempotency_keys (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    idempotency_key TEXT NOT NULL,
    user_id         UUID REFERENCES users (id) ON DELETE CASCADE,
    request_hash    TEXT NOT NULL,
    response_status INT NOT NULL,
    response_body   JSONB NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (idempotency_key, user_id)
);

-- ============================================================
-- Estructura académica: Curso -> Versión -> Módulo -> Unidad -> Recurso
-- ============================================================

CREATE TABLE courses (
    id                          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    teacher_id                  UUID NOT NULL REFERENCES users (id) ON DELETE RESTRICT,
    slug                        CITEXT NOT NULL UNIQUE,
    current_published_version_id UUID,
    created_at                  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at                  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_courses_teacher_id ON courses (teacher_id);

CREATE TABLE course_versions (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    course_id           UUID NOT NULL REFERENCES courses (id) ON DELETE CASCADE,
    version_number      INT NOT NULL,
    status              TEXT NOT NULL DEFAULT 'draft'
                            CHECK (status IN ('draft', 'published', 'unpublished', 'archived')),
    title               TEXT NOT NULL,
    summary             TEXT NOT NULL DEFAULT '',
    description_md      TEXT NOT NULL DEFAULT '',
    category            TEXT,
    level               TEXT,
    language            TEXT NOT NULL DEFAULT 'es',
    approval_min_score  NUMERIC(5, 2) NOT NULL DEFAULT 60.00,
    approval_required_resources_pct NUMERIC(5, 2) NOT NULL DEFAULT 100.00,
    published_at        TIMESTAMPTZ,
    created_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (course_id, version_number)
);

CREATE INDEX idx_course_versions_course_id ON course_versions (course_id);
CREATE INDEX idx_course_versions_status ON course_versions (status);

ALTER TABLE courses
    ADD CONSTRAINT fk_courses_current_published_version
    FOREIGN KEY (current_published_version_id) REFERENCES course_versions (id) ON DELETE SET NULL;

CREATE TABLE modules (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    course_version_id UUID NOT NULL REFERENCES course_versions (id) ON DELETE CASCADE,
    stable_id         UUID NOT NULL DEFAULT gen_random_uuid(),
    title             TEXT NOT NULL,
    position          INT NOT NULL,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (course_version_id, position)
);

CREATE INDEX idx_modules_course_version_id ON modules (course_version_id);
CREATE INDEX idx_modules_stable_id ON modules (stable_id);

CREATE TABLE units (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    module_id  UUID NOT NULL REFERENCES modules (id) ON DELETE CASCADE,
    stable_id  UUID NOT NULL DEFAULT gen_random_uuid(),
    title      TEXT NOT NULL,
    position   INT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (module_id, position)
);

CREATE INDEX idx_units_module_id ON units (module_id);
CREATE INDEX idx_units_stable_id ON units (stable_id);

CREATE TABLE resources (
    id                 UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    unit_id            UUID NOT NULL REFERENCES units (id) ON DELETE CASCADE,
    stable_id          UUID NOT NULL DEFAULT gen_random_uuid(),
    type               TEXT NOT NULL CHECK (type IN
                           ('text', 'image', 'video', 'audio', 'pdf', 'presentation',
                            'file', 'iframe', 'link', 'quiz')),
    title              TEXT NOT NULL,
    position           INT NOT NULL,
    visible            BOOLEAN NOT NULL DEFAULT true,
    required           BOOLEAN NOT NULL DEFAULT true,
    downloadable       BOOLEAN NOT NULL DEFAULT false,
    processing_status  TEXT NOT NULL DEFAULT 'none'
                           CHECK (processing_status IN ('none', 'pending', 'processing', 'ready', 'failed')),
    text_content_md    TEXT,
    external_url       TEXT,
    object_key         TEXT,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (unit_id, position)
);

CREATE INDEX idx_resources_unit_id ON resources (unit_id);
CREATE INDEX idx_resources_stable_id ON resources (stable_id);

-- ============================================================
-- Multimedia
-- ============================================================

CREATE TABLE media_assets (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    resource_id         UUID NOT NULL REFERENCES resources (id) ON DELETE CASCADE,
    original_object_key TEXT NOT NULL,
    mime_type           TEXT,
    size_bytes          BIGINT,
    checksum_sha256     TEXT,
    status              TEXT NOT NULL DEFAULT 'uploaded'
                            CHECK (status IN ('uploaded', 'scanning', 'queued', 'processing', 'ready', 'failed')),
    hls_master_key      TEXT,
    failure_reason      TEXT,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_media_assets_resource_id ON media_assets (resource_id);

CREATE TABLE upload_sessions (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id        UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    resource_id    UUID REFERENCES resources (id) ON DELETE CASCADE,
    object_key     TEXT NOT NULL,
    upload_id      TEXT NOT NULL,
    mime_type      TEXT,
    bytes_expected BIGINT,
    status         TEXT NOT NULL DEFAULT 'in_progress'
                       CHECK (status IN ('in_progress', 'completed', 'aborted', 'expired')),
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at     TIMESTAMPTZ NOT NULL
);

CREATE INDEX idx_upload_sessions_user_id ON upload_sessions (user_id);

-- ============================================================
-- Inscripciones y progreso
-- ============================================================

CREATE TABLE enrollments (
    id                 UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    student_id         UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    course_id          UUID NOT NULL REFERENCES courses (id) ON DELETE CASCADE,
    course_version_id  UUID NOT NULL REFERENCES course_versions (id) ON DELETE RESTRICT,
    status             TEXT NOT NULL DEFAULT 'active'
                           CHECK (status IN ('active', 'withdrawn', 'completed', 'approved')),
    enrolled_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    withdrawn_at       TIMESTAMPTZ,
    completed_at       TIMESTAMPTZ,
    approved_at        TIMESTAMPTZ,
    UNIQUE (student_id, course_id)
);

CREATE INDEX idx_enrollments_student_id ON enrollments (student_id);
CREATE INDEX idx_enrollments_course_id ON enrollments (course_id);

CREATE TABLE progress_events (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    enrollment_id     UUID NOT NULL REFERENCES enrollments (id) ON DELETE CASCADE,
    resource_stable_id UUID NOT NULL,
    event_type        TEXT NOT NULL CHECK (event_type IN ('open', 'heartbeat', 'close')),
    duration_seconds  INT NOT NULL DEFAULT 0,
    occurred_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_progress_events_enrollment_id ON progress_events (enrollment_id);

CREATE TABLE resource_progress (
    id                 UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    enrollment_id      UUID NOT NULL REFERENCES enrollments (id) ON DELETE CASCADE,
    resource_stable_id UUID NOT NULL,
    status             TEXT NOT NULL DEFAULT 'not_started'
                           CHECK (status IN ('not_started', 'in_progress', 'completed')),
    time_spent_seconds INT NOT NULL DEFAULT 0,
    first_opened_at    TIMESTAMPTZ,
    completed_at       TIMESTAMPTZ,
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (enrollment_id, resource_stable_id)
);

CREATE INDEX idx_resource_progress_enrollment_id ON resource_progress (enrollment_id);

-- ============================================================
-- Quizzes
-- ============================================================

CREATE TABLE quizzes (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    resource_id         UUID NOT NULL UNIQUE REFERENCES resources (id) ON DELETE CASCADE,
    title               TEXT NOT NULL,
    time_limit_seconds  INT,
    max_attempts        INT,
    pass_score          NUMERIC(5, 2) NOT NULL DEFAULT 60.00,
    feedback_policy     TEXT NOT NULL DEFAULT 'after_submit'
                            CHECK (feedback_policy IN ('immediate', 'after_submit', 'after_close', 'none')),
    shuffle_questions   BOOLEAN NOT NULL DEFAULT true,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE quiz_questions (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    quiz_id    UUID NOT NULL REFERENCES quizzes (id) ON DELETE CASCADE,
    stable_id  UUID NOT NULL DEFAULT gen_random_uuid(),
    position   INT NOT NULL,
    prompt_md  TEXT NOT NULL,
    type       TEXT NOT NULL DEFAULT 'single' CHECK (type IN ('single', 'multiple')),
    points     NUMERIC(6, 2) NOT NULL DEFAULT 1.00,
    UNIQUE (quiz_id, position)
);

CREATE TABLE quiz_options (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    question_id UUID NOT NULL REFERENCES quiz_questions (id) ON DELETE CASCADE,
    stable_id   UUID NOT NULL DEFAULT gen_random_uuid(),
    position    INT NOT NULL,
    text_md     TEXT NOT NULL,
    is_correct  BOOLEAN NOT NULL DEFAULT false,
    UNIQUE (question_id, position)
);

CREATE TABLE quiz_attempts (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    quiz_id          UUID NOT NULL REFERENCES quizzes (id) ON DELETE CASCADE,
    enrollment_id    UUID NOT NULL REFERENCES enrollments (id) ON DELETE CASCADE,
    attempt_number   INT NOT NULL,
    status           TEXT NOT NULL DEFAULT 'in_progress'
                         CHECK (status IN ('in_progress', 'submitted', 'expired')),
    snapshot         JSONB NOT NULL,
    started_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    submitted_at     TIMESTAMPTZ,
    expires_at       TIMESTAMPTZ,
    score            NUMERIC(6, 2),
    passed           BOOLEAN,
    submit_idempotency_key TEXT,
    UNIQUE (quiz_id, enrollment_id, attempt_number)
);

CREATE UNIQUE INDEX idx_quiz_attempts_idempotency
    ON quiz_attempts (quiz_id, enrollment_id, submit_idempotency_key)
    WHERE submit_idempotency_key IS NOT NULL;

CREATE TABLE quiz_answers (
    id                        UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    attempt_id                UUID NOT NULL REFERENCES quiz_attempts (id) ON DELETE CASCADE,
    question_stable_id        UUID NOT NULL,
    selected_option_stable_ids JSONB NOT NULL DEFAULT '[]'::jsonb,
    saved_at                  TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (attempt_id, question_stable_id)
);

-- ============================================================
-- Insignias
-- ============================================================

CREATE TABLE badges (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    enrollment_id     UUID NOT NULL UNIQUE REFERENCES enrollments (id) ON DELETE CASCADE,
    course_id         UUID NOT NULL REFERENCES courses (id) ON DELETE CASCADE,
    student_id        UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    verification_code TEXT NOT NULL UNIQUE,
    image_object_key  TEXT,
    issued_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    revoked_at        TIMESTAMPTZ,
    revoked_reason    TEXT
);

CREATE INDEX idx_badges_student_id ON badges (student_id);
