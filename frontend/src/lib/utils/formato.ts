const LOCALE = "es-CO";

export function formatearFecha(iso: string): string {
  return new Intl.DateTimeFormat(LOCALE, { dateStyle: "long" }).format(new Date(iso));
}

/** Fecha y hora, para listados donde el momento exacto importa. */
export function formatearFechaHora(iso: string): string {
  return new Intl.DateTimeFormat(LOCALE, {
    dateStyle: "medium",
    timeStyle: "short",
  }).format(new Date(iso));
}

export function formatearPorcentaje(valor: number): string {
  return new Intl.NumberFormat(LOCALE, {
    style: "percent",
    maximumFractionDigits: 0,
  }).format(valor / 100);
}
