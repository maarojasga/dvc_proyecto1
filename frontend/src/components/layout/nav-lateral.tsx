import Link from "next/link";
import type { EnlaceNav } from "@/lib/config/nav";

export function NavLateral({
  titulo,
  enlaces,
}: {
  titulo: string;
  enlaces: EnlaceNav[];
}) {
  return (
    <nav aria-label={titulo} className="sm:w-56 sm:shrink-0">
      <p className="mb-2 text-xs font-semibold uppercase tracking-wide text-texto-suave">
        {titulo}
      </p>
      <ul className="space-y-1 text-sm">
        {enlaces.map((enlace) => (
          <li key={enlace.href}>
            <Link
              href={enlace.href}
              className="block rounded-md px-2 py-1.5 text-texto-suave hover:bg-superficie hover:text-texto"
            >
              {enlace.etiqueta}
            </Link>
          </li>
        ))}
      </ul>
    </nav>
  );
}
