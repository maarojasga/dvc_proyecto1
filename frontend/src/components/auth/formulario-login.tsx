"use client";

import { useRouter } from "next/navigation";
import { useState } from "react";
import Link from "next/link";
import { api } from "@/lib/api/client";
import { endpoints } from "@/lib/api/endpoints";
import { ApiError } from "@/lib/api/errors";
import { Aviso, Boton, Campo } from "@/components/ui/formulario";
import { useEnvio } from "@/components/auth/use-envio";

/**
 * Inicio de sesion.
 *
 * El token de sesion no pasa por aqui: la API lo emite como cookie httpOnly al
 * responder. Tras entrar se refresca el arbol de servidor para que la cabecera
 * y las paginas protegidas vean la sesion nueva.
 */
export function FormularioLogin({ destino = "/mis-cursos" }: { destino?: string }) {
  const router = useRouter();
  const [correo, setCorreo] = useState("");
  const [clave, setClave] = useState("");
  const [pendienteVerificacion, setPendienteVerificacion] = useState(false);
  const { cargando, errorGeneral, erroresCampo, enviar } = useEnvio();

  return (
    <form
      className="space-y-4"
      noValidate
      onSubmit={(e) => {
        e.preventDefault();
        setPendienteVerificacion(false);
        void enviar(async () => {
          try {
            await api.post(endpoints.auth.login, { correo, clave });
          } catch (error) {
            // Cuenta sin verificar: se ofrece reenviar el enlace en lugar de
            // dejar al usuario atascado en un 403 sin salida.
            if (error instanceof ApiError && error.problem?.type.endsWith("correo-no-verificado")) {
              setPendienteVerificacion(true);
            }
            throw error;
          }
          router.replace(destino);
          router.refresh();
        });
      }}
    >
      {errorGeneral ? <Aviso tono="error">{errorGeneral}</Aviso> : null}
      {pendienteVerificacion ? (
        <Aviso tono="info">
          Tu cuenta aun no esta verificada.{" "}
          <Link href="/verificar-correo" className="underline">
            Reenviar el enlace de verificacion
          </Link>
          .
        </Aviso>
      ) : null}

      <Campo
        etiqueta="Correo"
        tipo="email"
        nombre="correo"
        valor={correo}
        onChange={setCorreo}
        errores={erroresCampo.correo}
        autoComplete="email"
      />
      <Campo
        etiqueta="Clave"
        tipo="password"
        nombre="clave"
        valor={clave}
        onChange={setClave}
        errores={erroresCampo.clave}
        autoComplete="current-password"
      />

      <div className="flex items-center justify-between gap-4">
        <Boton cargando={cargando}>{cargando ? "Entrando…" : "Entrar"}</Boton>
        <Link href="/recuperar-clave" className="text-sm text-texto-suave underline">
          Olvide mi clave
        </Link>
      </div>
    </form>
  );
}
