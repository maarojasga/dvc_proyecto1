import Link from "next/link";
import { Container } from "@/components/ui/container";
import { navPrincipal } from "@/lib/config/nav";
import { obtenerUsuario } from "@/lib/auth/session";
import { BotonSalir } from "@/components/auth/boton-salir";

/**
 * Cabecera global.
 *
 * Los enlaces se filtran por rol para no ofrecer destinos que la API va a
 * rechazar. El filtrado es de presentacion: la autorizacion real la aplica
 * cada endpoint segun rol, propiedad e inscripcion.
 */
export async function SiteHeader() {
  const usuario = await obtenerUsuario();

  const enlaces = navPrincipal.filter(
    (enlace) =>
      !enlace.roles ||
      enlace.roles.some((rol) => usuario?.roles.includes(rol)),
  );

  return (
    <header className="border-b border-borde">
      <Container className="flex h-16 items-center justify-between gap-6">
        <Link href="/" className="text-sm font-semibold tracking-tight">
          Plataforma MOOC
        </Link>
        <nav aria-label="Navegacion principal">
          <ul className="flex items-center gap-4 text-sm">
            {enlaces.map((enlace) => (
              <li key={enlace.href}>
                <Link
                  href={enlace.href}
                  className="text-texto-suave hover:text-texto"
                >
                  {enlace.etiqueta}
                </Link>
              </li>
            ))}
            {usuario ? (
              <>
                <li>
                  <Link
                    href="/cuenta/sesiones"
                    className="text-texto-suave hover:text-texto"
                  >
                    {usuario.nombre}
                  </Link>
                </li>
                <li>
                  <BotonSalir />
                </li>
              </>
            ) : (
              <li>
                <Link href="/login" className="text-texto-suave hover:text-texto">
                  Iniciar sesion
                </Link>
              </li>
            )}
          </ul>
        </nav>
      </Container>
    </header>
  );
}
