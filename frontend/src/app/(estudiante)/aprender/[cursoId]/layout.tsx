import type { ReactNode } from "react";

/**
 * Marco del visor de curso. Alojara el indice de modulos y unidades junto al
 * contenido del recurso activo.
 */
export default function LayoutAprender({ children }: { children: ReactNode }) {
  return <div className="flex flex-col gap-8 sm:flex-row">{children}</div>;
}
