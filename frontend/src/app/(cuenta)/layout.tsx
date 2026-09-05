import type { ReactNode } from "react";
import { redirect } from "next/navigation";
import { Container } from "@/components/ui/container";
import { obtenerUsuario } from "@/lib/auth/session";

/**
 * Area de cuenta: exige sesion.
 *
 * La comprobacion aqui evita pintar una pantalla vacia a quien no ha entrado,
 * pero no es la defensa: cada endpoint de la API verifica la sesion por su
 * cuenta, asi que saltarse esta redireccion no da acceso a ningun dato.
 */
export default async function LayoutCuenta({ children }: { children: ReactNode }) {
  if (!(await obtenerUsuario())) {
    redirect("/login");
  }
  return <Container className="max-w-3xl space-y-6">{children}</Container>;
}
