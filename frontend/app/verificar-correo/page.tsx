"use client";

import { Suspense, useEffect, useState } from "react";
import { useSearchParams } from "next/navigation";
import Link from "next/link";
import { api, ApiError } from "@/lib/api";

function VerifyEmailInner() {
  const params = useSearchParams();
  const token = params.get("token") ?? "";
  const [status, setStatus] = useState<"pending" | "ok" | "error">("pending");
  const [message, setMessage] = useState("");

  useEffect(() => {
    if (!token) {
      setStatus("error");
      setMessage("Falta el token de verificación en el enlace.");
      return;
    }
    api
      .verifyEmail(token)
      .then(() => setStatus("ok"))
      .catch((e) => {
        setStatus("error");
        setMessage(e instanceof ApiError ? e.message : "No se pudo verificar el correo");
      });
  }, [token]);

  return (
    <div className="columna-estrecha">
      <h1>Verificación de correo</h1>
      {status === "pending" && <p>Verificando…</p>}
      {status === "ok" && (
        <>
          <p className="success-banner">Tu correo fue verificado. Ya puedes iniciar sesión.</p>
          <Link href="/login">Ir a iniciar sesión</Link>
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
    <Suspense fallback={<p>Cargando…</p>}>
      <VerifyEmailInner />
    </Suspense>
  );
}
