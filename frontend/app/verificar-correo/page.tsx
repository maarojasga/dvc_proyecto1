"use client";

import { Suspense, useEffect, useState } from "react";
import { useSearchParams } from "next/navigation";
import Link from "next/link";
import { api, ApiError } from "@/lib/api";
import { useI18n, useTraductorEstable } from "@/lib/i18n";

function VerifyEmailInner() {
  const params = useSearchParams();
  const token = params.get("token") ?? "";
  const [status, setStatus] = useState<"pending" | "ok" | "error">("pending");
  const [message, setMessage] = useState("");
  const { t } = useI18n();
  // El token es de un solo uso: el efecto no puede relanzarse al cambiar de
  // idioma, así que traduce con un traductor de identidad estable.
  const traducir = useTraductorEstable();

  useEffect(() => {
    if (!token) {
      setStatus("error");
      setMessage(traducir("auth.verificar.sinToken"));
      return;
    }
    api
      .verifyEmail(token)
      .then(() => setStatus("ok"))
      .catch((e) => {
        setStatus("error");
        setMessage(e instanceof ApiError ? e.message : traducir("auth.verificar.error"));
      });
  }, [token, traducir]);

  return (
    <div className="columna-estrecha">
      <h1>{t("auth.verificar.titulo")}</h1>
      {status === "pending" && <p>{t("auth.verificar.enCurso")}</p>}
      {status === "ok" && (
        <>
          <p className="success-banner">{t("auth.verificar.ok")}</p>
          <Link href="/login">{t("auth.irALogin")}</Link>
        </>
      )}
      {status === "error" && (
        <p className="error-banner" role="alert">
          {message}
        </p>
      )}
    </div>
  );
}

export default function VerifyEmailPage() {
  return (
    <Suspense fallback={<Cargando />}>
      <VerifyEmailInner />
    </Suspense>
  );
}

/**
 * El fallback de Suspense también se traduce, y por eso es un componente: el
 * contexto de idioma solo se lee dentro de uno.
 */
function Cargando() {
  const { t } = useI18n();
  return <p>{t("comun.cargando")}</p>;
}
