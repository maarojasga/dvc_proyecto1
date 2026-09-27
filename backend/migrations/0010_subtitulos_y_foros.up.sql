-- Subtitulos, transcripciones y foros asincronos (alcance opcional 5.2).

-- Una pista de subtitulos por recurso e idioma.
--
-- El archivo WebVTT vive en el almacen de objetos, como todo binario. La
-- transcripcion se guarda aqui, en texto: se deriva de la pista al subirla y
-- es lo que hace el material buscable y legible sin reproducirlo.
CREATE TABLE resource_captions (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    resource_id  UUID NOT NULL REFERENCES resources (id) ON DELETE CASCADE,
    -- Etiqueta BCP 47 ("es", "en", "pt-BR").
    language     TEXT NOT NULL,
    label        TEXT NOT NULL,
    object_key   TEXT NOT NULL,
    transcript   TEXT NOT NULL DEFAULT '',
    -- subtitles traduce dialogo; captions describe ademas los sonidos
    -- relevantes, que es lo que necesita quien no oye.
    kind         TEXT NOT NULL DEFAULT 'subtitles'
                     CHECK (kind IN ('subtitles', 'captions')),
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (resource_id, language)
);

CREATE INDEX idx_resource_captions_recurso ON resource_captions (resource_id);

-- Foros asincronos.
--
-- Los hilos cuelgan del curso y no de la version: una discusion sobre una
-- leccion sigue valiendo cuando el profesor publica una actualizacion. Cuando
-- el hilo es sobre un recurso concreto se guarda su stable_id, que es
-- justamente el identificador que sobrevive a las versiones.
CREATE TABLE forum_threads (
    id                 UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    course_id          UUID NOT NULL REFERENCES courses (id) ON DELETE CASCADE,
    resource_stable_id UUID,
    author_id          UUID REFERENCES users (id) ON DELETE SET NULL,
    title              TEXT NOT NULL,
    body_md            TEXT NOT NULL,
    locked             BOOLEAN NOT NULL DEFAULT false,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    -- Ordenar por actividad exige saber cuando fue la ultima; calcularlo con
    -- un max() sobre las respuestas en cada listado obligaria a recorrerlas.
    last_activity_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_forum_threads_curso ON forum_threads (course_id, last_activity_at DESC);
CREATE INDEX idx_forum_threads_recurso ON forum_threads (course_id, resource_stable_id);

CREATE TABLE forum_posts (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    thread_id  UUID NOT NULL REFERENCES forum_threads (id) ON DELETE CASCADE,
    author_id  UUID REFERENCES users (id) ON DELETE SET NULL,
    body_md    TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    -- Borrar una respuesta la oculta pero no la quita: en una conversacion,
    -- eliminar un mensaje intermedio deja sin sentido a los que respondian.
    deleted_at TIMESTAMPTZ
);

CREATE INDEX idx_forum_posts_hilo ON forum_posts (thread_id, created_at);
