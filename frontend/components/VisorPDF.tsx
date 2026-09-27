"use client";

import { useI18n } from "@/lib/i18n";

/**
 * Visor de PDF.
 *
 * Usa el visor integrado del navegador en lugar de renderizar el documento a
 * mano: ya trae búsqueda, zoom, navegación por teclado y lectura de la capa de
 * texto, que es justo lo que costaría más reproducir de forma accesible.
 *
 * El enlace de debajo no es decorativo: si el navegador no puede incrustar
 * PDF, es la única forma de llegar al documento, así que se muestra siempre.
 */
export function VisorPDF({
  src,
  titulo,
  descargable,
}: {
  src: string;
  titulo: string;
  descargable: boolean;
}) {
  const { t } = useI18n();
  return (
    <div className="stack">
      <iframe
        src={src}
        title={t("pdf.documento", { titulo })}
        style={{
          width: "100%",
          height: "min(75vh, 900px)",
          border: "1px solid var(--color-border)",
          borderRadius: "var(--radius)",
          background: "var(--color-surface)",
        }}
      />
      <p className="muted">
        <a href={src} target="_blank" rel="noopener noreferrer">
          {t("pdf.abrir")}
        </a>
        {descargable ? t("pdf.descargable") : t("pdf.noDescargable")}
      </p>
    </div>
  );
}
