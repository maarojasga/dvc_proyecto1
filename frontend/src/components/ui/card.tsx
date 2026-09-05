import type { ReactNode } from "react";
import { cn } from "@/lib/utils/cn";

export function Card({
  children,
  className,
}: {
  children: ReactNode;
  className?: string;
}) {
  return (
    <div
      className={cn(
        "rounded-lg border border-borde bg-superficie p-5",
        className,
      )}
    >
      {children}
    </div>
  );
}

export function CardTitle({ children }: { children: ReactNode }) {
  return <h2 className="text-base font-semibold">{children}</h2>;
}

export function CardText({ children }: { children: ReactNode }) {
  return <p className="mt-2 text-sm text-texto-suave">{children}</p>;
}
