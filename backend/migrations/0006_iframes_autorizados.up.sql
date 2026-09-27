-- Lista blanca de destinos incrustables (alcance opcional 5.2).
--
-- Un iframe carga código de terceros dentro de la pagina del curso. La lista
-- es la unica capa que impide que ese contenido llegue siquiera al navegador:
-- el sandbox y la politica de permisos lo contienen una vez cargado, pero no
-- deciden si se carga.

CREATE TABLE iframe_allowlist (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    host                TEXT NOT NULL UNIQUE,
    include_subdomains  BOOLEAN NOT NULL DEFAULT false,
    -- Sintaxis del atributo `allow` del iframe ("fullscreen; autoplay").
    -- Vacio, que es lo normal, no concede ninguna funcionalidad del
    -- navegador al contenido incrustado.
    permissions         TEXT NOT NULL DEFAULT '',
    description         TEXT NOT NULL DEFAULT '',
    created_by          UUID REFERENCES users (id) ON DELETE SET NULL,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);

COMMENT ON COLUMN iframe_allowlist.include_subdomains IS
    'Extiende la autorizacion a los subdominios del host. Va explicito porque '
    'autorizar *.example.com regala cualquier subdominio que el tercero cree '
    'despues, incluido el que aloje contenido de sus usuarios.';

-- Semilla de destinos habituales en material didactico. Son una comodidad
-- para arrancar, no una recomendacion: la administracion puede borrarlos.
--
-- youtube-nocookie y no youtube: el dominio con cookies rastrea al estudiante
-- desde la pagina del curso, lo que convierte incrustar un video en compartir
-- su navegacion con un tercero.
INSERT INTO iframe_allowlist (host, include_subdomains, permissions, description) VALUES
    ('www.youtube-nocookie.com', false, 'fullscreen; picture-in-picture',
     'YouTube en modo sin cookies'),
    ('player.vimeo.com', false, 'fullscreen; picture-in-picture',
     'Reproductor de Vimeo'),
    ('docs.google.com', false, 'fullscreen',
     'Documentos y presentaciones publicadas de Google'),
    ('www.geogebra.org', false, 'fullscreen',
     'Applets interactivos de GeoGebra');
