import type { ReactNode } from "react";
import { Container } from "@/components/ui/container";

export default function LayoutProfesor({ children }: { children: ReactNode }) {
  return <Container className="space-y-6">{children}</Container>;
}
