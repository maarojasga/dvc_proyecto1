-- La bitácora debe ser inmutable: el criterio de evaluación pide auditoría
-- inmutable, y una tabla que admite UPDATE y DELETE solo lo es por convenio.
-- El disparador lo hace cumplir en la propia base, de modo que ni un error de
-- la aplicación ni un acceso directo puedan reescribir el historial.

CREATE FUNCTION audit_logs_append_only() RETURNS trigger
    LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'audit_logs es inmutable: % no esta permitido', TG_OP;
END;
$$;

CREATE TRIGGER audit_logs_no_modifications
    BEFORE UPDATE OR DELETE ON audit_logs
    FOR EACH ROW EXECUTE FUNCTION audit_logs_append_only();
