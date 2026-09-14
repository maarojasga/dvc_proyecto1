-- Conversion de presentaciones PPTX y ODP a PDF (alcance opcional 5.2).
--
-- La vista previa es un derivado, no un reemplazo: el original que subio el
-- profesor se conserva, igual que con el video. Por eso la clave del PDF va en
-- su propia columna y no sobrescribe original_object_key.

ALTER TABLE media_assets ADD COLUMN derived_pdf_key TEXT;

COMMENT ON COLUMN media_assets.derived_pdf_key IS
    'PDF generado a partir de una presentacion, para previsualizarla con el '
    'visor de PDF. El original se conserva en original_object_key.';
