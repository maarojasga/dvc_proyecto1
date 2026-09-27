-- audit_logs_actor_user_id_fkey se declaro ON DELETE SET NULL: al borrar un
-- usuario, la fila de auditoria debe sobrevivir sin apuntar a nadie, en vez
-- de desaparecer con el. Pero el disparador de 0002 bloqueaba TODO UPDATE
-- sin excepcion, asi que ese propio SET NULL disparado por la base de datos
-- quedaba rechazado por su propio disparador de inmutabilidad: borrar
-- cualquier usuario con historial de auditoria era imposible.
--
-- Se reemplaza la funcion para permitir exactamente ese caso -actor_user_id
-- pasando de un valor a NULL, sin que ninguna otra columna cambie- y seguir
-- rechazando cualquier otra modificacion, incluido poner actor_user_id de
-- vuelta a un valor no nulo.
CREATE OR REPLACE FUNCTION audit_logs_append_only() RETURNS trigger
    LANGUAGE plpgsql AS $$
BEGIN
    IF TG_OP = 'UPDATE'
       AND OLD.actor_user_id IS NOT NULL
       AND NEW.actor_user_id IS NULL
       AND NEW.id = OLD.id
       AND NEW.action = OLD.action
       AND NEW.entity_type = OLD.entity_type
       AND NEW.entity_id IS NOT DISTINCT FROM OLD.entity_id
       AND NEW.metadata = OLD.metadata
       AND NEW.ip_address IS NOT DISTINCT FROM OLD.ip_address
       AND NEW.created_at = OLD.created_at
    THEN
        RETURN NEW;
    END IF;
    RAISE EXCEPTION 'audit_logs es inmutable: % no esta permitido', TG_OP;
END;
$$;
