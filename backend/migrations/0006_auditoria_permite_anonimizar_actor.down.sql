CREATE OR REPLACE FUNCTION audit_logs_append_only() RETURNS trigger
    LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'audit_logs es inmutable: % no esta permitido', TG_OP;
END;
$$;
