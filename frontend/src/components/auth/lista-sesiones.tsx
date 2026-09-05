"use client";

import { useCallback, useEffect, useState } from "react";
import { useRouter } from "next/navigation";
import { api } from "@/lib/api/client";
import { endpoints } from "@/lib/api/endpoints";
import { Aviso, Boton } from "@/components/ui/formulario";
import { Card } from "@/components/ui/card";
import { useEnvio } from "@/components/auth/use-envio";
import { formatearFechaHora } from "@/lib/utils/formato";
import type { PaginaCursor } from "@/types";

interface SesionActiva {
  id: string;
  creadaEn: string;
  ultimoUsoEn: string;
  expiraEn: string;
  ip?: string;
  userAgent?: string;
  actual: boolean;
}

/**
 * Sesiones activas del usuario, con revocacion.
 *
 * Revocar la sesion en curso equivale a salir, asi que en ese caso se vuelve
 * al inicio en lugar de recargar una lista a la que ya no hay acceso.
 */
export function ListaSesiones() {
  const router = useRouter();
  const [sesiones, setSesiones] = useState<SesionActiva[] | null>(null);
  const { cargando, errorGeneral, enviar } = useEnvio();

  const cargar = useCallback(async () => {
    const pagina = await api.get<PaginaCursor<SesionActiva>>(endpoints.auth.sesiones);
    setSesiones(pagina.items);
  }, []);

  useEffect(() => {
    void enviar(cargar);
  }, [enviar, cargar]);

  if (errorGeneral) {
    return <Aviso tono="error">{errorGeneral}</Aviso>;
  }
  if (sesiones === null) {
    return (
      <p role="status" className="text-sm text-texto-suave">
        Cargando sesiones…
      </p>
    );
  }

  const otras = sesiones.filter((s) => !s.actual).length;

  return (
    <div className="space-y-4">
      <ul className="space-y-3">
        {sesiones.map((sesion) => (
          <li key={sesion.id}>
            <Card className="flex flex-wrap items-start justify-between gap-4">
              <div className="space-y-1 text-sm">
                <p className="font-medium">
                  {sesion.actual ? "Este dispositivo" : "Otro dispositivo"}
                </p>
                <p className="text-texto-suave">
                  Ultimo uso: {formatearFechaHora(sesion.ultimoUsoEn)}
                </p>
                <p className="text-texto-suave">
                  Iniciada: {formatearFechaHora(sesion.creadaEn)}
                  {sesion.ip ? ` · ${sesion.ip}` : ""}
                </p>
                {sesion.userAgent ? (
                  <p className="break-all text-xs text-texto-suave">{sesion.userAgent}</p>
                ) : null}
              </div>
              <Boton
                tipo="button"
                variante="secundario"
                cargando={cargando}
                onClick={() =>
                  void enviar(async () => {
                    await api.delete(endpoints.auth.sesion(sesion.id));
                    if (sesion.actual) {
                      router.replace("/");
                      router.refresh();
                      return;
                    }
                    await cargar();
                  })
                }
              >
                {sesion.actual ? "Cerrar esta sesion" : "Revocar"}
              </Boton>
            </Card>
          </li>
        ))}
      </ul>

      {otras > 0 ? (
        <Boton
          tipo="button"
          variante="secundario"
          cargando={cargando}
          onClick={() =>
            void enviar(async () => {
              await api.delete(endpoints.auth.sesiones);
              await cargar();
            })
          }
        >
          Cerrar las demas sesiones ({otras})
        </Boton>
      ) : null}
    </div>
  );
}
