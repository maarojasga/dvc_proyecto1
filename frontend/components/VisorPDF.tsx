"use client";

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
  return (
    <div className="stack">
      <iframe
        src={src}
        title={`Documento: ${titulo}`}
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
          Abrir el documento en una pestaña nueva
        </a>
        {descargable
          ? " · Desde ahí puedes descargarlo."
          : " · El autor no habilitó la descarga."}
      </p>
    </div>
  );
}
