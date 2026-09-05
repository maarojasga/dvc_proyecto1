import type { ReactNode } from "react";

export function PageHeader({
  titulo,
  descripcion,
  acciones,
}: {
  titulo: string;
  descripcion?: string;
  acciones?: ReactNode;
}) {
  return (
    <header className="flex flex-col gap-3 border-b border-borde pb-6 sm:flex-row sm:items-end sm:justify-between">
      <div className="space-y-2">
        <h1 className="text-2xl font-semibold tracking-tight sm:text-3xl">{titulo}</h1>
        {descripcion ? (
          <p className="max-w-2xl text-sm text-texto-suave">{descripcion}</p>
        ) : null}
      </div>
      {acciones ? <div className="flex gap-2">{acciones}</div> : null}
    </header>
  );
}
