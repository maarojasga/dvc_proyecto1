"use client";

import { useEffect } from "react";
import { api } from "@/lib/api";

// Dentro de la ventana [5s, 90s] que el servidor exige entre heartbeats
// (backend/internal/domain/progress/progress.go): fuera de ese rango la senal
// se descarta por implausible.
const HEARTBEAT_MS = 20_000;

/**
 * Reporta senales de progreso verificado por servidor sobre un recurso: abre
 * al montar, late cada 20 segundos mientras la pestana esta visible, y cierra
 * al salir (cambio de pestana, navegacion o desmontaje).
 *
 * El servidor decide cuanto tiempo acredita comparando sus propias marcas de
 * tiempo, y si eso alcanza para completar el recurso; aqui nunca se calcula
 * ni se envia ningun porcentaje, porque un valor asi lo rechazaria la API.
 */
export function useProgressReporting(resourceId: string | undefined) {
  useEffect(() => {
    if (!resourceId) return;
    // Se captura en una constante porque las funciones declaradas mas abajo
    // se ejecutan de forma diferida (temporizador, eventos), y TypeScript no
    // extiende el estrechamiento de `string | undefined` a closures que
    // capturan un parametro de funcion.
    const id = resourceId;

    void api.recordProgress(id, "open").catch(() => undefined);

    const intervalo = setInterval(() => {
      if (document.visibilityState === "visible") {
        void api.recordProgress(id, "heartbeat").catch(() => undefined);
      }
    }, HEARTBEAT_MS);

    // `complete` no es una orden de completar: es una peticion al servidor de
    // que reevalue si el tiempo acumulado ya alcanza. Sobre un recurso que no
    // se mide por tiempo (texto, imagen, enlace), abrirlo y cerrarlo basta.
    function reportarCierre() {
      void api.recordProgress(id, "close", true, { keepalive: true }).catch(() => undefined);
    }

    function alCambiarVisibilidad() {
      if (document.visibilityState === "hidden") reportarCierre();
    }

    document.addEventListener("visibilitychange", alCambiarVisibilidad);
    window.addEventListener("pagehide", reportarCierre);

    return () => {
      clearInterval(intervalo);
      document.removeEventListener("visibilitychange", alCambiarVisibilidad);
      window.removeEventListener("pagehide", reportarCierre);
      reportarCierre();
    };
  }, [resourceId]);
}
