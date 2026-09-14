-- Coautoria basica (alcance opcional 5.2).
--
-- El curso conserva un unico dueno en courses.teacher_id: es quien responde
-- por el y quien puede repartir el acceso. Los colaboradores editan, pero no
-- pueden anadir ni quitar a otros, que es lo que distingue "coautoria basica"
-- de una gestion de permisos completa.

CREATE TABLE course_collaborators (
    course_id  UUID NOT NULL REFERENCES courses (id) ON DELETE CASCADE,
    user_id    UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    -- Un solo rol por ahora. La columna existe para que anadir "revisor" (que
    -- comenta pero no edita) no obligue a migrar la tabla entera.
    role       TEXT NOT NULL DEFAULT 'editor' CHECK (role IN ('editor')),
    added_by   UUID REFERENCES users (id) ON DELETE SET NULL,
    added_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (course_id, user_id)
);

CREATE INDEX idx_course_collaborators_usuario ON course_collaborators (user_id);

COMMENT ON TABLE course_collaborators IS
    'Profesores que pueden editar un curso ajeno. El dueno sigue siendo '
    'courses.teacher_id: los colaboradores no reparten acceso.';
