"use client";

import { Suspense, useState } from "react";
import { useSearchParams } from "next/navigation";
import Link from "next/link";
import { api, ApiError } from "@/lib/api";

function RequestResetForm() {
  const [email, setEmail] = useState("");
  const [done, setDone] = useState(false);
  const [submitting, setSubmitting] = useState(false);

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    setSubmitting(true);
    await api.requestPasswordReset(email).catch(() => undefined);
    setSubmitting(false);
    setDone(true);
  }

  if (done) {
    return (
      <p className="success-banner">
        Si el correo existe, enviamos un enlace para restablecer la contraseña. Revisa Mailpit en desarrollo
        (http://localhost:8025).
      </p>
    );
  }

  return (
    <form onSubmit={handleSubmit} className="stack">
      <div className="form-field">
        <label htmlFor="email">Correo electrónico</label>
        <input id="email" type="email" required value={email} onChange={(e) => setEmail(e.target.value)} />
      </div>
      <button type="submit" disabled={submitting}>
        {submitting ? "Enviando…" : "Enviar enlace"}
      </button>
    </form>
  );
}

function ConfirmResetForm({ token }: { token: string }) {
  const [password, setPassword] = useState("");
  const [error, setError] = useState("");
  const [done, setDone] = useState(false);
  const [submitting, setSubmitting] = useState(false);

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    setError("");
    setSubmitting(true);
    try {
      await api.confirmPasswordReset(token, password);
      setDone(true);
    } catch (e) {
      setError(e instanceof ApiError ? e.message : "No se pudo restablecer la contraseña");
    } finally {
      setSubmitting(false);
    }
  }

  if (done) {
    return (
      <>
        <p className="success-banner">Tu contraseña fue actualizada. Todas tus sesiones anteriores fueron cerradas.</p>
        <Link href="/login">Ir a iniciar sesión</Link>
      </>
    );
  }

  return (
    <form onSubmit={handleSubmit} className="stack">
      {error && (
        <p className="error-banner" role="alert">
          {error}
        </p>
      )}
      <div className="form-field">
        <label htmlFor="password">Nueva contraseña</label>
        <input
          id="password"
          type="password"
          required
          minLength={10}
          value={password}
          onChange={(e) => setPassword(e.target.value)}
        />
      </div>
      <button type="submit" disabled={submitting}>
        {submitting ? "Guardando…" : "Restablecer contraseña"}
      </button>
    </form>
  );
}

function ResetPasswordInner() {
  const params = useSearchParams();
  const token = params.get("token");

  return (
    <div className="columna-estrecha">
      <h1>Restablecer contraseña</h1>
      {token ? <ConfirmResetForm token={token} /> : <RequestResetForm />}
    </div>
  );
}

export default function ResetPasswordPage() {
  return (
    <Suspense fallback={<p>Cargando…</p>}>
      <ResetPasswordInner />
    </Suspense>
  );
}
