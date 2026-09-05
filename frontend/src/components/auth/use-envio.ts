"use client";

import { useCallback, useState } from "react";
import { ApiError } from "@/lib/api/errors";

/**
 * Estado compartido por los formularios de identidad.
 *
 * Se llama useEnvio y no usarEnvio porque React exige el prefijo `use` para
 * reconocerlo como hook y aplicarle sus reglas.
 *
 * Centraliza el envio para que cada pantalla solo declare que llamada hace y
 * como se traduce el resultado, y para que el manejo de errores sea igual en
 * todas: los errores por campo van al campo, el resto al aviso general.
 */
export function useEnvio() {
  const [cargando, setCargando] = useState(false);
  const [errorGeneral, setErrorGeneral] = useState<string | null>(null);
  const [erroresCampo, setErroresCampo] = useState<Record<string, string[]>>({});

  // Identidad estable: los formularios que llaman a enviar desde un efecto
  // dependen de ella para no reejecutarlo en cada render.
  const enviar = useCallback(async (accion: () => Promise<void>) => {
    setCargando(true);
    setErrorGeneral(null);
    setErroresCampo({});
    try {
      await accion();
    } catch (error) {
      if (error instanceof ApiError) {
        const porCampo = error.erroresPorCampo;
        setErroresCampo(porCampo);
        // Si el error es solo de campos, no se duplica arriba el mensaje.
        if (Object.keys(porCampo).length === 0) {
          setErrorGeneral(error.problem?.detail ?? error.message);
        }
      } else {
        setErrorGeneral("No se pudo contactar con el servidor. Intentalo de nuevo.");
      }
    } finally {
      setCargando(false);
    }
  }, []);

  return { cargando, errorGeneral, erroresCampo, enviar };
}
