"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { api, type ResumenProgreso } from "@/lib/api";

/**
 * Latidos de progreso.
 *
 * El avance lo determina el servidor con estas señales: abrí el recurso, sigo
 * aquí, lo cerré. El cliente no manda porcentajes —el servidor los rechaza y
 * los audita— y tampoco decide si algo cuenta como completado: solo informa de
 * hechos.
 *
 * El intervalo es un contrato con el servidor, que descarta los latidos que
 * llegan antes del mínimo plausible. Enviar más rápido no acelera el avance:
 * lo marca como manipulación.
 */
const INTERVALO_LATIDO_MS = 30_000;

export function useLatidosDeProgreso(courseId: string, resourceId: string, activo: boolean) {
  const [resumen, setResumen] = useState<ResumenProgreso | null>(null);
  // El cierre se manda una sola vez. Sin esta guarda, ocultar la pestaña y
  // luego navegar enviaría dos.
  const cerrado = useRef(false);

  useEffect(() => {
    if (!activo) return;
    let vivo = true;
    cerrado.current = false;

    // Un fallo al reportar no debe estropear la lectura del material: el
    // estudiante está aprendiendo, y el progreso se recupera en el siguiente
    // latido.
    const enviar = (tipo: "open" | "heartbeat") =>
      api
        .registrarProgreso(resourceId, tipo)
        .then((r) => {
          if (vivo) setResumen(r);
        })
        .catch(() => undefined);

    /**
     * cerrar avisa de que el estudiante dejó el recurso. Es la señal que lo
     * acredita en el material que no se mide por tiempo (texto, imagen,
     * enlace), así que perderla significa que un recurso leído nunca cuenta
     * como visto y el curso no se puede terminar.
     *
     * Va con keepalive porque una petición normal lanzada mientras la página
     * se descarga la cancela el navegador. Y se manda también en pagehide y al
     * ocultarse la pestaña, porque la limpieza de un efecto no se ejecuta
     * cuando alguien cierra la pestaña o el móvil manda el navegador al fondo.
     */
    const cerrar = () => {
      if (cerrado.current) return;
      cerrado.current = true;
      void api.registrarProgreso(resourceId, "close", false, true).catch(() => undefined);
    };

    const alOcultarse = () => {
      if (document.visibilityState === "hidden") cerrar();
    };

    void enviar("open");
    const t = setInterval(() => {
      // Si la pestaña no está visible, nadie está consumiendo el recurso.
      // Seguir latiendo sería reportar permanencia que no ocurre.
      if (document.visibilityState === "visible") void enviar("heartbeat");
    }, INTERVALO_LATIDO_MS);

    window.addEventListener("pagehide", cerrar);
    document.addEventListener("visibilitychange", alOcultarse);

    return () => {
      vivo = false;
      clearInterval(t);
      window.removeEventListener("pagehide", cerrar);
      document.removeEventListener("visibilitychange", alOcultarse);
      cerrar();
    };
  }, [resourceId, activo]);

  /**
   * refrescar vuelve a leer el avance del curso. Lo usa el cuestionario al
   * enviar: aprobar una evaluación cambia el avance y el número que se estaba
   * mostrando queda obsoleto.
   *
   * Es una lectura pura a propósito. Volver a ejecutar el ciclo de latidos
   * mandaría un cierre y una apertura de más, y esas señales son justo la
   * evidencia con la que el servidor calcula el avance: ensuciarlas para
   * repintar un número sería cambiar la medida por mostrarla.
   */
  const refrescar = useCallback(() => {
    api
      .progresoDeCurso(courseId)
      .then(setResumen)
      .catch(() => undefined);
  }, [courseId]);

  return { resumen, refrescar };
}
