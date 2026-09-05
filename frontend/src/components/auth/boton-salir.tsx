"use client";

import { useRouter } from "next/navigation";
import { useState } from "react";
import { api } from "@/lib/api/client";
import { endpoints } from "@/lib/api/endpoints";

/** Cierra la sesion en curso y refresca el arbol de servidor. */
export function BotonSalir() {
  const router = useRouter();
  const [cargando, setCargando] = useState(false);

  return (
    <button
      type="button"
      disabled={cargando}
      aria-busy={cargando || undefined}
      onClick={() => {
        setCargando(true);
        void (async () => {
          try {
            await api.post(endpoints.auth.logout);
          } finally {
            // Aunque la revocacion falle, se recarga: si la sesion ya no vale,
            // el servidor lo reflejara.
            setCargando(false);
            router.replace("/");
            router.refresh();
          }
        })();
      }}
      className="text-texto-suave underline hover:text-texto disabled:opacity-60"
    >
      {cargando ? "Saliendo…" : "Salir"}
    </button>
  );
}
