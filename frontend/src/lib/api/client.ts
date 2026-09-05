import { env } from "@/lib/config/env";
import { ApiError } from "@/lib/api/errors";
import type { OpcionesPeticion, ProblemDetails } from "@/types";

/**
 * Cliente REST minimo contra la API en Go.
 *
 * Es la base de transporte: aun no hay llamadas reales en la aplicacion.
 * Deja resueltas las convenciones obligatorias del proyecto (Idempotency-Key,
 * ETag, errores uniformes) para que cada modulo solo declare sus endpoints.
 */

function construirUrl(ruta: string, query?: OpcionesPeticion["query"]): string {
  const base = typeof window === "undefined" ? env.apiInternalUrl : env.apiBaseUrl;
  const url = `${base.replace(/\/$/, "")}/${ruta.replace(/^\//, "")}`;
  if (!query) return url;

  const params = new URLSearchParams();
  for (const [clave, valor] of Object.entries(query)) {
    if (valor !== undefined) params.set(clave, String(valor));
  }
  const cadena = params.toString();
  return cadena ? `${url}?${cadena}` : url;
}

export async function peticion<T>(
  ruta: string,
  { idempotencyKey, etag, query, headers, ...init }: OpcionesPeticion = {},
): Promise<T> {
  const cabeceras = new Headers(headers);
  cabeceras.set("Accept", "application/json");
  if (init.body && !cabeceras.has("Content-Type")) {
    cabeceras.set("Content-Type", "application/json");
  }
  if (idempotencyKey) cabeceras.set("Idempotency-Key", idempotencyKey);
  if (etag) cabeceras.set("If-Match", etag);

  const respuesta = await fetch(construirUrl(ruta, query), {
    ...init,
    headers: cabeceras,
    // Las sesiones viajan en cookie httpOnly emitida por la API.
    credentials: "include",
  });

  if (respuesta.status === 204) return undefined as T;

  const esJson = respuesta.headers
    .get("Content-Type")
    ?.includes("json");
  const cuerpo: unknown = esJson ? await respuesta.json() : await respuesta.text();

  if (!respuesta.ok) {
    const problem = esJson ? (cuerpo as ProblemDetails) : undefined;
    throw new ApiError(
      respuesta.status,
      problem?.title ?? `La API respondio ${respuesta.status}`,
      problem,
    );
  }

  return cuerpo as T;
}

export const api = {
  get: <T>(ruta: string, opciones?: OpcionesPeticion) =>
    peticion<T>(ruta, { ...opciones, method: "GET" }),
  post: <T>(ruta: string, cuerpo?: unknown, opciones?: OpcionesPeticion) =>
    peticion<T>(ruta, {
      ...opciones,
      method: "POST",
      body: cuerpo === undefined ? undefined : JSON.stringify(cuerpo),
    }),
  put: <T>(ruta: string, cuerpo?: unknown, opciones?: OpcionesPeticion) =>
    peticion<T>(ruta, {
      ...opciones,
      method: "PUT",
      body: cuerpo === undefined ? undefined : JSON.stringify(cuerpo),
    }),
  patch: <T>(ruta: string, cuerpo?: unknown, opciones?: OpcionesPeticion) =>
    peticion<T>(ruta, {
      ...opciones,
      method: "PATCH",
      body: cuerpo === undefined ? undefined : JSON.stringify(cuerpo),
    }),
  delete: <T>(ruta: string, opciones?: OpcionesPeticion) =>
    peticion<T>(ruta, { ...opciones, method: "DELETE" }),
};
