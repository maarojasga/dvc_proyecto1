import type { ReactNode } from "react";

/** Pildora informativa (estado de curso, rol, estado de procesamiento). */
export function Etiqueta({ children }: { children: ReactNode }) {
  return (
    <span className="inline-flex items-center rounded-full border border-borde px-2.5 py-0.5 text-xs font-medium text-texto-suave">
      {children}
    </span>
  );
}
