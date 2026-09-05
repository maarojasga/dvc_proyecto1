import type { ReactNode } from "react";
import { NavLateral } from "@/components/layout/nav-lateral";

export default async function LayoutCursoAutoria({
  children,
  params,
}: {
  children: ReactNode;
  params: Promise<{ cursoId: string }>;
}) {
  const { cursoId } = await params;
  const base = `/autoria/cursos/${cursoId}`;

  return (
    <div className="flex flex-col gap-8 sm:flex-row">
      <NavLateral
        titulo="Curso"
        enlaces={[
          { href: base, etiqueta: "Metadatos" },
          { href: `${base}/estructura`, etiqueta: "Estructura" },
          { href: `${base}/versiones`, etiqueta: "Versiones" },
        ]}
      />
      <div className="flex-1 space-y-6">{children}</div>
    </div>
  );
}
