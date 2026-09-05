"use client";

import type { ReactNode } from "react";
import { useId } from "react";
import { cn } from "@/lib/utils/cn";

/**
 * Primitivas de formulario accesibles.
 *
 * Cada campo lleva etiqueta asociada, el error se anuncia con aria-describedby
 * y aria-invalid, y el resumen de error del formulario es una region viva para
 * que un lector de pantalla lo lea sin mover el foco.
 */

export function Campo({
  etiqueta,
  tipo = "text",
  nombre,
  valor,
  onChange,
  errores,
  ayuda,
  requerido = true,
  autoComplete,
  minLength,
}: {
  etiqueta: string;
  tipo?: "text" | "email" | "password";
  nombre: string;
  valor: string;
  onChange: (valor: string) => void;
  errores?: string[];
  ayuda?: string;
  requerido?: boolean;
  autoComplete?: string;
  minLength?: number;
}) {
  const id = useId();
  const idAyuda = `${id}-ayuda`;
  const idError = `${id}-error`;
  const tieneError = Boolean(errores?.length);

  const descritoPor = [ayuda ? idAyuda : null, tieneError ? idError : null]
    .filter(Boolean)
    .join(" ");

  return (
    <div className="space-y-1.5">
      <label htmlFor={id} className="block text-sm font-medium">
        {etiqueta}
      </label>
      <input
        id={id}
        name={nombre}
        type={tipo}
        value={valor}
        onChange={(e) => onChange(e.target.value)}
        required={requerido}
        autoComplete={autoComplete}
        minLength={minLength}
        aria-invalid={tieneError || undefined}
        aria-describedby={descritoPor || undefined}
        className={cn(
          "w-full rounded-md border bg-superficie px-3 py-2 text-sm",
          "focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-acento",
          tieneError ? "border-error" : "border-borde",
        )}
      />
      {ayuda ? (
        <p id={idAyuda} className="text-xs text-texto-suave">
          {ayuda}
        </p>
      ) : null}
      {tieneError ? (
        <p id={idError} className="text-xs text-error">
          {errores?.join(" ")}
        </p>
      ) : null}
    </div>
  );
}

export function Boton({
  children,
  tipo = "submit",
  cargando = false,
  variante = "primario",
  onClick,
}: {
  children: ReactNode;
  tipo?: "submit" | "button";
  cargando?: boolean;
  variante?: "primario" | "secundario";
  onClick?: () => void;
}) {
  return (
    <button
      type={tipo}
      onClick={onClick}
      disabled={cargando}
      // aria-busy anuncia el envio en curso sin depender solo del texto.
      aria-busy={cargando || undefined}
      className={cn(
        "inline-flex items-center justify-center rounded-md px-4 py-2 text-sm font-medium",
        "focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-acento",
        "disabled:cursor-not-allowed disabled:opacity-60",
        variante === "primario"
          ? "bg-acento text-acento-contraste hover:opacity-90"
          : "border border-borde hover:bg-superficie-alta",
      )}
    >
      {children}
    </button>
  );
}

/** Aviso de formulario: error general o confirmacion. */
export function Aviso({
  tono,
  titulo,
  children,
}: {
  tono: "error" | "exito" | "info";
  titulo?: string;
  children: ReactNode;
}) {
  return (
    <div
      // Los errores interrumpen; las confirmaciones esperan turno.
      role={tono === "error" ? "alert" : "status"}
      aria-live={tono === "error" ? "assertive" : "polite"}
      className={cn(
        "rounded-md border px-4 py-3 text-sm",
        tono === "error" && "border-error bg-error-suave text-error",
        tono === "exito" && "border-exito bg-exito-suave text-exito",
        tono === "info" && "border-borde bg-superficie-alta text-texto-suave",
      )}
    >
      {titulo ? <p className="font-medium">{titulo}</p> : null}
      <div className={titulo ? "mt-1" : undefined}>{children}</div>
    </div>
  );
}
