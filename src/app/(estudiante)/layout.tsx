import type { ReactNode } from "react";
import { Container } from "@/components/ui/container";

/**
 * Area de estudiante. La verificacion de sesion e inscripcion es responsabilidad
 * del servidor; aqui solo se define el marco visual.
 */
export default function LayoutEstudiante({ children }: { children: ReactNode }) {
  return <Container className="space-y-6">{children}</Container>;
}
