"use client";

import { useEffect, useRef } from "react";

import { useI18n } from "@/lib/i18n";

/** PistaDeSubtitulos es un <track> ya resuelto por el servidor. */
export interface PistaDeSubtitulos {
  language: string;
  label: string;
  kind: "subtitles" | "captions";
  url: string;
}

/**
 * Reproductor adaptativo de video y audio sobre HLS.
 *
 * Safari reproduce HLS de forma nativa; el resto de navegadores necesitan
 * hls.js, que además es quien cambia de calidad según el ancho de banda. Se
 * carga de forma diferida para no arrastrar la librería en las páginas que no
 * reproducen nada.
 *
 * Se usan los controles nativos a propósito: ya son operables por teclado y
 * los conoce cualquier lector de pantalla, cosa que un control propio tendría
 * que ganarse desde cero.
 */
export function ReproductorHLS({
  src,
  tipo,
  posicionInicial = 0,
  onPosicion,
  titulo,
  pistas = [],
}: {
  src: string;
  tipo: "video" | "audio";
  /** Segundo donde reanudar. */
  posicionInicial?: number;
  /** Se llama con la posición actual para que el llamador la reporte. */
  onPosicion?: (segundos: number) => void;
  titulo: string;
  /** Subtítulos disponibles, ya autorizados por el servidor. */
  pistas?: PistaDeSubtitulos[];
}) {
  const { t } = useI18n();
  const ref = useRef<HTMLMediaElement | null>(null);
  // El callback se guarda en una referencia para que cambiarlo no reinicie la
  // reproducción: el efecto solo debe rehacerse si cambia la fuente.
  const alReportar = useRef(onPosicion);
  alReportar.current = onPosicion;

  useEffect(() => {
    const media = ref.current;
    if (!media) return;

    let hls: import("hls.js").default | null = null;
    let cancelado = false;

    const situarse = () => {
      if (posicionInicial > 0 && Number.isFinite(media.duration)) {
        // Nunca al final exacto: reanudar ahí deja el reproductor parado.
        media.currentTime = Math.min(posicionInicial, Math.max(media.duration - 5, 0));
      }
    };
    media.addEventListener("loadedmetadata", situarse, { once: true });

    if (media.canPlayType("application/vnd.apple.mpegurl")) {
      media.src = src;
    } else {
      void import("hls.js").then(({ default: Hls }) => {
        if (cancelado || !Hls.isSupported()) {
          if (!cancelado) media.src = src;
          return;
        }
        hls = new Hls({ enableWorker: true });
        hls.loadSource(src);
        hls.attachMedia(media as HTMLVideoElement);
      });
    }

    // La posición se reporta con cuentagotas: sirve para reanudar, no para
    // seguir el avance, así que no hace falta precisión de segundo.
    const intervalo = window.setInterval(() => {
      if (!media.paused && media.currentTime > 0) {
        alReportar.current?.(Math.floor(media.currentTime));
      }
    }, 15000);

    const alPausar = () => {
      if (media.currentTime > 0) alReportar.current?.(Math.floor(media.currentTime));
    };
    media.addEventListener("pause", alPausar);

    return () => {
      cancelado = true;
      window.clearInterval(intervalo);
      media.removeEventListener("pause", alPausar);
      media.removeEventListener("loadedmetadata", situarse);
      hls?.destroy();
    };
  }, [src, posicionInicial]);

  if (tipo === "audio") {
    return (
      <audio
        ref={ref as React.RefObject<HTMLAudioElement>}
        controls
        preload="metadata"
        aria-label={t("media.audio", { titulo })}
        style={{ width: "100%" }}
      >
        {pistas.map((p) => (
          <track key={p.language} kind={p.kind} src={p.url} srcLang={p.language} label={p.label} />
        ))}
      </audio>
    );
  }

  return (
    <video
      ref={ref as React.RefObject<HTMLVideoElement>}
      controls
      playsInline
      preload="metadata"
      aria-label={t("media.video", { titulo })}
      style={{ width: "100%", maxHeight: "70vh", background: "#000", borderRadius: "var(--radius)" }}
    >
      {/* Los subtítulos van como <track> nativos: el navegador ya sabe
          mostrarlos, dejar elegir idioma y respetar las preferencias de
          accesibilidad del sistema. Un renderizador propio tendría que
          ganarse todo eso desde cero. */}
      {pistas.map((p) => (
        <track key={p.language} kind={p.kind} src={p.url} srcLang={p.language} label={p.label} />
      ))}
    </video>
  );
}
