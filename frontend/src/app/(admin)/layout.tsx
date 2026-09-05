import type { ReactNode } from "react";
import { Container } from "@/components/ui/container";
import { NavLateral } from "@/components/layout/nav-lateral";
import { navAdmin } from "@/lib/config/nav";

export default function LayoutAdmin({ children }: { children: ReactNode }) {
  return (
    <Container>
      <div className="flex flex-col gap-8 sm:flex-row">
        <NavLateral titulo="Administracion" enlaces={navAdmin} />
        <div className="flex-1 space-y-6">{children}</div>
      </div>
    </Container>
  );
}
