-- Historial visible de revisiones del editor (alcance opcional 5.2).
--
-- El autoguardado del editor conserva el borrador en el navegador, que se
-- pierde al cambiar de equipo y no dice quien escribio que. El historial lo
-- guarda en el servidor: cada guardado deja una revision numerada, atribuida y
-- restaurable.
--
-- Se guarda el Markdown canonico entero y no un diff. Un curso son kilobytes
-- de texto, asi que el ahorro seria irrelevante, mientras que reconstruir una
-- version aplicando diffs encadenados es justo lo que falla cuando mas falta
-- hace: al recuperar algo que se borro por error.

CREATE TABLE resource_revisions (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    resource_id     UUID NOT NULL REFERENCES resources (id) ON DELETE CASCADE,
    revision_number INT NOT NULL,
    content_md      TEXT NOT NULL,
    -- El autor se conserva aunque la cuenta se borre: quien escribio que es
    -- justamente lo que hace util el historial en un curso con coautoria.
    author_id       UUID REFERENCES users (id) ON DELETE SET NULL,
    author_email    TEXT NOT NULL DEFAULT '',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (resource_id, revision_number)
);

CREATE INDEX idx_resource_revisions_recurso
    ON resource_revisions (resource_id, revision_number DESC);
