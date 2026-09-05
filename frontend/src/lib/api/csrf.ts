/**
 * Defensa anti-CSRF por doble envio de cookie.
 *
 * La API emite `mooc_csrf` legible por el navegador junto con la cookie de
 * sesion, que es httpOnly. Toda peticion que cambie estado debe repetir ese
 * valor en la cabecera: un sitio atacante puede provocar la peticion, pero no
 * puede leer la cookie para rellenar la cabecera.
 */

export const NOMBRE_COOKIE_CSRF = "mooc_csrf";
export const CABECERA_CSRF = "X-CSRF-Token";

/** Lee el token anti-CSRF de las cookies del navegador. */
export function tokenCsrf(): string | undefined {
  if (typeof document === "undefined") return undefined;
  const entrada = document.cookie
    .split("; ")
    .find((c) => c.startsWith(`${NOMBRE_COOKIE_CSRF}=`));
  return entrada ? decodeURIComponent(entrada.slice(NOMBRE_COOKIE_CSRF.length + 1)) : undefined;
}
