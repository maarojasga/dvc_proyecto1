-- Un recurso tiene un solo activo multimedia.
--
-- Confirmar la misma carga dos veces creaba un activo nuevo cada vez, y la
-- clave de idempotencia de la cola era ese identificador, así que Asynq
-- publicaba dos transcodificaciones del mismo original. El índice único cierra
-- esa carrera: reutilizar el activo existente hace que las dos confirmaciones
-- publiquen el mismo trabajo.

DELETE FROM media_assets
 WHERE id IN (
   SELECT id FROM (
     SELECT id, ROW_NUMBER() OVER (
       PARTITION BY resource_id
       ORDER BY
         CASE status
           WHEN 'ready' THEN 0
           WHEN 'processing' THEN 1
           WHEN 'queued' THEN 2
           WHEN 'uploaded' THEN 3
           ELSE 4
         END,
         created_at DESC
     ) AS n
     FROM media_assets
   ) duplicados
   WHERE n > 1
 );

DROP INDEX IF EXISTS idx_media_assets_resource_id;

CREATE UNIQUE INDEX idx_media_assets_un_recurso ON media_assets (resource_id);
