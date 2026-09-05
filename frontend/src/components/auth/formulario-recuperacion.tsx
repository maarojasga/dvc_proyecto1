"use client";

import { useState } from "react";
import Link from "next/link";
import { api } from "@/lib/api/client";
import { endpoints } from "@/lib/api/endpoints";
import { Aviso, Boton, Campo } from "@/components/ui/formulario";
import { useEnvio } from "@/components/auth/use-envio";

/**
 * Recuperacion de clave, en sus dos momentos.
 *
 * Sin token en la URL se pide el correo; con token se fija la clave nueva.
 * Confirmar cierra todas las sesiones del usuario, asi que el mensaje final lo
 * anuncia: quien recupera puede estar expulsando a un intruso.
 */
export function FormularioRecuperacion({ token }: { token?: string }) {
  return token ? <FijarClave token={token} /> : <SolicitarEnlace />;
}

function SolicitarEnlace() {
  const [correo, setCorreo] = useState("");
  const [enviado, setEnviado] = useState(false);
  const { cargando, errorGeneral, erroresCampo, enviar } = useEnvio();

  if (enviado) {
    return (
      <Aviso tono="exito" titulo="Revisa tu correo">
        Si la direccion corresponde a una cuenta, enviamos un enlace para
        restablecer la clave. Vence en una hora y solo puede usarse una vez.
      </Aviso>
    );
  }

  return (
    <form
      className="space-y-4"
      noValidate
      onSubmit={(e) => {
        e.preventDefault();
        void enviar(async () => {
          await api.post(endpoints.auth.recuperarClave, { correo });
          setEnviado(true);
        });
      }}
    >
      {errorGeneral ? <Aviso tono="error">{errorGeneral}</Aviso> : null}
      <Campo
        etiqueta="Correo"
        tipo="email"
        nombre="correo"
        valor={correo}
        onChange={setCorreo}
        errores={erroresCampo.correo}
        autoComplete="email"
      />
      <Boton cargando={cargando}>{cargando ? "Enviando…" : "Enviar enlace"}</Boton>
    </form>
  );
}

function FijarClave({ token }: { token: string }) {
  const [clave, setClave] = useState("");
  const [listo, setListo] = useState(false);
  const { cargando, errorGeneral, erroresCampo, enviar } = useEnvio();

  if (listo) {
    return (
      <Aviso tono="exito" titulo="Clave actualizada">
        Cerramos todas las sesiones abiertas por seguridad.{" "}
        <Link href="/login" className="underline">
          Inicia sesion
        </Link>{" "}
        con la clave nueva.
      </Aviso>
    );
  }

  return (
    <form
      className="space-y-4"
      noValidate
      onSubmit={(e) => {
        e.preventDefault();
        void enviar(async () => {
          await api.post(endpoints.auth.confirmarRecuperacion, { token, clave });
          setListo(true);
        });
      }}
    >
      {errorGeneral ? <Aviso tono="error">{errorGeneral}</Aviso> : null}
      <Aviso tono="info">
        Al fijar la clave nueva se cerraran todas las sesiones de la cuenta.
      </Aviso>
      <Campo
        etiqueta="Clave nueva"
        tipo="password"
        nombre="clave"
        valor={clave}
        onChange={setClave}
        errores={erroresCampo.clave}
        ayuda="Entre 10 y 128 caracteres."
        autoComplete="new-password"
        minLength={10}
      />
      <Boton cargando={cargando}>{cargando ? "Guardando…" : "Guardar clave"}</Boton>
    </form>
  );
}
