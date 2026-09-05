import type { ReactNode } from "react";
import { Container } from "@/components/ui/container";

export default function LayoutAuth({ children }: { children: ReactNode }) {
  return (
    <Container className="max-w-md space-y-6">{children}</Container>
  );
}
