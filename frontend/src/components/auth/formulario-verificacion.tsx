"use client";

import { useEffect, useRef, useState } from "react";
import Link from "next/link";
import { api } from "@/lib/api/client";
import { endpoints } from "@/lib/api/endpoints";
import { Aviso, Boton, Campo } from "@/components/ui/formulario";
import { useEnvio } from "@/components/auth/use-envio";

/**
 * Verificacion del correo.
 *
 * Si la URL trae el token del enlace, se consume solo al montar. Si no, se
 * ofrece reenviar el enlace, que es el caso de quien llega desde el login con
 * la cuenta pendiente.
 */
export function FormularioVerificacion({ token }: { token?: string }) {
  const [verificado, setVerificado] = useState(false);
  const [reenviado, setReenviado] = useState(false);
  const [correo, setCorreo] = useState("");
  const { cargando, errorGeneral, erroresCampo, enviar } = useEnvio();
  const yaIntentado = useRef(false);

  useEffect(() => {
    // El token es de un solo uso: en desarrollo React monta dos veces, y sin
    // esta guarda el segundo intento consumiria un token ya gastado y
    // mostraria un error falso.
    if (!token || yaIntentado.current) return;
    yaIntentado.current = true;
    void enviar(async () => {
      await api.post(endpoints.auth.verificarCorreo, { token });
      setVerificado(true);
    });
  }, [token, enviar]);

  if (verificado) {
    return (
      <Aviso tono="exito" titulo="Cuenta verificada">
        Ya puedes{" "}
        <Link href="/login" className="underline">
          iniciar sesion
        </Link>
        .
      </Aviso>
    );
  }

  if (token) {
    if (cargando) {
      return (
        <Aviso tono="info">
          <span role="status">Verificando el enlace…</span>
        </Aviso>
      );
    }
    return (
      <div className="space-y-4">
        <Aviso tono="error" titulo="No se pudo verificar">
          {errorGeneral ?? "El enlace no es valido o ya fue utilizado."}
        </Aviso>
        <p className="text-sm text-texto-suave">
          Pide un enlace nuevo con el formulario de abajo.
        </p>
        <FormularioReenvio
          correo={correo}
          setCorreo={setCorreo}
          cargando={cargando}
          erroresCampo={erroresCampo}
          reenviado={reenviado}
          onEnviar={() =>
            void enviar(async () => {
              await api.post(endpoints.auth.reenviarVerificacion, { correo });
              setReenviado(true);
            })
          }
        />
      </div>
    );
  }

  return (
    <div className="space-y-4">
      {errorGeneral ? <Aviso tono="error">{errorGeneral}</Aviso> : null}
      <FormularioReenvio
        correo={correo}
        setCorreo={setCorreo}
        cargando={cargando}
        erroresCampo={erroresCampo}
        reenviado={reenviado}
        onEnviar={() =>
          void enviar(async () => {
            await api.post(endpoints.auth.reenviarVerificacion, { correo });
            setReenviado(true);
          })
        }
      />
    </div>
  );
}

function FormularioReenvio({
  correo,
  setCorreo,
  cargando,
  erroresCampo,
  reenviado,
  onEnviar,
}: {
  correo: string;
  setCorreo: (v: string) => void;
  cargando: boolean;
  erroresCampo: Record<string, string[]>;
  reenviado: boolean;
  onEnviar: () => void;
}) {
  if (reenviado) {
    return (
      <Aviso tono="exito" titulo="Enlace enviado">
        Si la cuenta existe y esta pendiente, enviamos un enlace nuevo.
      </Aviso>
    );
  }
  return (
    <form
      className="space-y-4"
      noValidate
      onSubmit={(e) => {
        e.preventDefault();
        onEnviar();
      }}
    >
      <Campo
        etiqueta="Correo"
        tipo="email"
        nombre="correo"
        valor={correo}
        onChange={setCorreo}
        errores={erroresCampo.correo}
        autoComplete="email"
      />
      <Boton cargando={cargando}>
        {cargando ? "Enviando…" : "Reenviar enlace de verificacion"}
      </Boton>
    </form>
  );
}
