"use client";

import { Container } from "@/components/ui/container";

export default function ErrorGlobal({ reset }: { error: Error; reset: () => void }) {
  return (
    <Container className="space-y-4">
      <h1 className="text-2xl font-semibold">Ocurrio un error</h1>
      <p className="text-texto-suave">
        No fue posible completar la operacion. Intenta nuevamente.
      </p>
      <button
        type="button"
        onClick={reset}
        className="rounded-md bg-acento px-4 py-2 text-sm font-medium text-acento-contraste"
      >
        Reintentar
      </button>
    </Container>
  );
}
