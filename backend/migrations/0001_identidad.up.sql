-- Identidad y auditoria: usuarios, roles, sesiones revocables, tokens de un
-- solo uso y bitacora inmutable.
--
-- El correo se guarda como text y se normaliza (recorte + minusculas) en el
-- dominio antes de persistir, de modo que UNIQUE basta y no hace falta la
-- extension citext.

CREATE TABLE usuarios (
    id                    uuid        PRIMARY KEY,
    correo                text        NOT NULL UNIQUE,
    nombre                text        NOT NULL,
    clave_hash            text        NOT NULL,
    estado                text        NOT NULL
        CHECK (estado IN ('pendiente', 'activo', 'suspendido')),
    correo_verificado_en  timestamptz,
    creado_en             timestamptz NOT NULL DEFAULT now(),
    actualizado_en        timestamptz NOT NULL DEFAULT now()
);

-- Los roles son globales. El registro publico solo crea 'estudiante'; los
-- profesores y administradores se otorgan por administracion.
CREATE TABLE usuario_roles (
    usuario_id uuid NOT NULL REFERENCES usuarios (id) ON DELETE CASCADE,
    rol        text NOT NULL
        CHECK (rol IN ('administrador', 'profesor', 'estudiante')),
    PRIMARY KEY (usuario_id, rol)
);

-- Sesiones revocables. Postgres es la fuente de verdad; Redis solo acelera la
-- lectura, de modo que revocar aqui invalida la sesion aunque la cache falle.
CREATE TABLE sesiones (
    id            uuid        PRIMARY KEY,
    usuario_id    uuid        NOT NULL REFERENCES usuarios (id) ON DELETE CASCADE,
    token_hash    bytea       NOT NULL UNIQUE,
    creada_en     timestamptz NOT NULL DEFAULT now(),
    expira_en     timestamptz NOT NULL,
    ultimo_uso_en timestamptz NOT NULL DEFAULT now(),
    revocada_en   timestamptz,
    ip            text,
    user_agent    text
);

CREATE INDEX sesiones_activas_por_usuario
    ON sesiones (usuario_id) WHERE revocada_en IS NULL;

-- Verificacion de correo y recuperacion de clave. Se guarda solo el hash del
-- token: quien lea la tabla no puede usarlo.
CREATE TABLE tokens_unico_uso (
    id         uuid        PRIMARY KEY,
    usuario_id uuid        NOT NULL REFERENCES usuarios (id) ON DELETE CASCADE,
    proposito  text        NOT NULL
        CHECK (proposito IN ('verificacion_correo', 'recuperacion_clave')),
    token_hash bytea       NOT NULL UNIQUE,
    creado_en  timestamptz NOT NULL DEFAULT now(),
    expira_en  timestamptz NOT NULL,
    usado_en   timestamptz
);

CREATE INDEX tokens_pendientes_por_usuario
    ON tokens_unico_uso (usuario_id, proposito) WHERE usado_en IS NULL;

CREATE TABLE auditoria (
    id          bigint      GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    ocurrido_en timestamptz NOT NULL DEFAULT now(),
    actor_id    uuid        REFERENCES usuarios (id) ON DELETE SET NULL,
    accion      text        NOT NULL,
    entidad     text,
    entidad_id  text,
    ip          text,
    user_agent  text,
    datos       jsonb       NOT NULL DEFAULT '{}'::jsonb
);

CREATE INDEX auditoria_por_fecha ON auditoria (ocurrido_en DESC);
CREATE INDEX auditoria_por_actor ON auditoria (actor_id, ocurrido_en DESC);

-- La auditoria es inmutable: solo admite INSERT.
CREATE FUNCTION auditoria_solo_insercion() RETURNS trigger
    LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'la auditoria es inmutable: % no esta permitido', TG_OP;
END;
$$;

CREATE TRIGGER auditoria_sin_modificaciones
    BEFORE UPDATE OR DELETE ON auditoria
    FOR EACH ROW EXECUTE FUNCTION auditoria_solo_insercion();
