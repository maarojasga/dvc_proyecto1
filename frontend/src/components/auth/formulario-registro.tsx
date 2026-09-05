"use client";

import { useState } from "react";
import { api } from "@/lib/api/client";
import { endpoints } from "@/lib/api/endpoints";
import { Aviso, Boton, Campo } from "@/components/ui/formulario";
import { useEnvio } from "@/components/auth/use-envio";

/**
 * Alta de estudiante.
 *
 * La API responde igual exista o no el correo, asi que la confirmacion se
 * redacta sin afirmar que la cuenta se creo: decirlo delataria que cuentas
 * existen.
 */
export function FormularioRegistro() {
  const [correo, setCorreo] = useState("");
  const [nombre, setNombre] = useState("");
  const [clave, setClave] = useState("");
  const [enviado, setEnviado] = useState(false);
  const { cargando, errorGeneral, erroresCampo, enviar } = useEnvio();

  if (enviado) {
    return (
      <Aviso tono="exito" titulo="Revisa tu correo">
        Si la direccion esta disponible, enviamos un enlace para verificar la
        cuenta. El enlace vence en 24 horas.
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
          await api.post(endpoints.auth.registro, { correo, nombre, clave });
          setEnviado(true);
        });
      }}
    >
      {errorGeneral ? <Aviso tono="error">{errorGeneral}</Aviso> : null}

      <Campo
        etiqueta="Nombre"
        nombre="nombre"
        valor={nombre}
        onChange={setNombre}
        errores={erroresCampo.nombre}
        autoComplete="name"
      />
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
        ayuda="Entre 10 y 128 caracteres."
        autoComplete="new-password"
        minLength={10}
      />

      <Boton cargando={cargando}>{cargando ? "Creando cuenta…" : "Crear cuenta"}</Boton>
    </form>
  );
}
