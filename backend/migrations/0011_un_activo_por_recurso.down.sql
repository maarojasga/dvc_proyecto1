DROP INDEX IF EXISTS idx_media_assets_un_recurso;

CREATE INDEX idx_media_assets_resource_id ON media_assets (resource_id);
