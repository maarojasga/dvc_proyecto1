import Link from "next/link";
import { Container } from "@/components/ui/container";
import { navPrincipal } from "@/lib/config/nav";

/**
 * Cabecera global. Muestra toda la navegacion sin filtrar por rol: el filtrado
 * llega cuando exista sesion real (ver src/lib/auth/session.ts).
 */
export function SiteHeader() {
  return (
    <header className="border-b border-borde">
      <Container className="flex h-16 items-center justify-between gap-6">
        <Link href="/" className="text-sm font-semibold tracking-tight">
          Plataforma MOOC
        </Link>
        <nav aria-label="Navegacion principal">
          <ul className="flex items-center gap-4 text-sm">
            {navPrincipal.map((enlace) => (
              <li key={enlace.href}>
                <Link
                  href={enlace.href}
                  className="text-texto-suave hover:text-texto"
                >
                  {enlace.etiqueta}
                </Link>
              </li>
            ))}
          </ul>
        </nav>
      </Container>
    </header>
  );
}
