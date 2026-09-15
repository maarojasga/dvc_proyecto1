-- Arrendamiento del procesamiento multimedia.
--
-- La idempotencia se apoyaba solo en comprobar si el activo ya estaba listo.
-- Eso no cubre la doble entrega mientras el primer intento sigue corriendo:
-- ambos veían el activo sin terminar y transcodificaban a la vez, subiendo
-- salidas duplicadas sobre las mismas claves.
--
-- Con esta marca, tomar el trabajo es un UPDATE condicional: solo un worker
-- pasa de un estado no terminal a 'processing'. Y si el que lo tomó muere, el
-- arrendamiento vence y otro puede recogerlo, en lugar de dejar el recurso
-- atascado para siempre.

ALTER TABLE media_assets
    ADD COLUMN processing_started_at TIMESTAMPTZ;

COMMENT ON COLUMN media_assets.processing_started_at IS
    'Instante en que un worker tomó el trabajo. Vencido el arrendamiento, otro worker puede reclamarlo.';
