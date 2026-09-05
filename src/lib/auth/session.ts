import type { Rol, Usuario } from "@/types";

/**
 * Lectura de la sesion actual.
 *
 * Placeholder de la base: todavia no consulta la API ni lee la cookie de sesion.
 * Cuando se implemente, debe resolverse en el servidor contra
 * endpoints.auth.sesionActual y nunca confiar en datos enviados por el cliente.
 */
export async function obtenerSesion(): Promise<Usuario | null> {
  return null;
}

export function tieneRol(usuario: Usuario | null, ...roles: Rol[]): boolean {
  if (!usuario) return false;
  return roles.some((rol) => usuario.roles.includes(rol));
}
