DROP TRIGGER IF EXISTS auditoria_sin_modificaciones ON auditoria;
DROP FUNCTION IF EXISTS auditoria_solo_insercion();
DROP TABLE IF EXISTS auditoria;
DROP TABLE IF EXISTS tokens_unico_uso;
DROP TABLE IF EXISTS sesiones;
DROP TABLE IF EXISTS usuario_roles;
DROP TABLE IF EXISTS usuarios;
