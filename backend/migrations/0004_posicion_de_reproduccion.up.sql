-- Última posición reportada de reproducción.
--
-- Es una comodidad para reanudar donde se dejó, no evidencia de avance: la
-- envía el cliente y por tanto no puede sostener ninguna decisión académica.
-- El progreso se calcula aparte, en el servidor, a partir de heartbeats y
-- eventos de apertura; por eso vive en su propia columna y no toca
-- time_spent_seconds ni status.

ALTER TABLE resource_progress
    ADD COLUMN last_position_seconds INT NOT NULL DEFAULT 0
        CHECK (last_position_seconds >= 0);

COMMENT ON COLUMN resource_progress.last_position_seconds IS
    'Segundo donde reanudar la reproduccion. Lo reporta el cliente: es una comodidad, no evidencia de progreso.';
