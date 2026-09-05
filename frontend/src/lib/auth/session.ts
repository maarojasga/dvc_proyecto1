import { cookies } from "next/headers";
import { env } from "@/lib/config/env";
import { endpoints } from "@/lib/api/endpoints";
import type { Rol, Usuario } from "@/types";

/**
 * Lectura de la sesion en el servidor.
 *
 * Se resuelve siempre contra la API, nunca a partir de datos del cliente: la
 * cookie de sesion es httpOnly y opaca, y quien decide si sigue siendo valida
 * es el backend, que puede haberla revocado hace un instante.
 */

/** Respuesta de GET /auth/sesion. */
interface RespuestaSesion {
  usuario: Usuario;
  sesionId: string;
}

/** Sesion resuelta, o null si no hay ninguna vigente. */
export async function obtenerSesion(): Promise<RespuestaSesion | null> {
  const cookieSesion = (await cookies()).toString();
  if (!cookieSesion) return null;

  try {
    const respuesta = await fetch(
      `${env.apiInternalUrl.replace(/\/$/, "")}${endpoints.auth.sesionActual}`,
      {
        headers: { cookie: cookieSesion, Accept: "application/json" },
        // La sesion cambia con cada revocacion: no debe cachearse.
        cache: "no-store",
      },
    );
    if (!respuesta.ok) return null;
    return (await respuesta.json()) as RespuestaSesion;
  } catch {
    // Si la API no responde, la pagina se pinta como anonima en lugar de
    // romperse: ninguna vista publica depende de la sesion.
    return null;
  }
}

/** Usuario de la sesion actual, o null. */
export async function obtenerUsuario(): Promise<Usuario | null> {
  return (await obtenerSesion())?.usuario ?? null;
}

export function tieneRol(usuario: Usuario | null, ...roles: Rol[]): boolean {
  if (!usuario) return false;
  return roles.some((rol) => usuario.roles.includes(rol));
}
