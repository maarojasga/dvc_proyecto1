"use client";

import { Suspense, useState } from "react";
import { useSearchParams } from "next/navigation";
import Link from "next/link";
import { api, ApiError } from "@/lib/api";
import { useI18n } from "@/lib/i18n";

function RequestResetForm() {
  const { t } = useI18n();
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
      <p className="success-banner">{t("auth.reset.enviado")}</p>
    );
  }

  return (
    <form onSubmit={handleSubmit} className="stack">
      <div className="form-field">
        <label htmlFor="email">{t("auth.correo")}</label>
        <input id="email" type="email" required value={email} onChange={(e) => setEmail(e.target.value)} />
      </div>
      <button type="submit" disabled={submitting}>
        {submitting ? t("auth.reset.enviando") : t("auth.reset.enviar")}
      </button>
    </form>
  );
}

function ConfirmResetForm({ token }: { token: string }) {
  const { t } = useI18n();
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
      setError(e instanceof ApiError ? e.message : t("auth.reset.error"));
    } finally {
      setSubmitting(false);
    }
  }

  if (done) {
    return (
      <>
        <p className="success-banner">{t("auth.reset.actualizada")}</p>
        <Link href="/login">{t("auth.irALogin")}</Link>
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
        <label htmlFor="password">{t("auth.reset.nueva")}</label>
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
        {submitting ? t("comun.guardando") : t("auth.reset.titulo")}
      </button>
    </form>
  );
}

function ResetPasswordInner() {
  const { t } = useI18n();
  const params = useSearchParams();
  const token = params.get("token");

  return (
    <div className="columna-estrecha">
      <h1>{t("auth.reset.titulo")}</h1>
      {token ? <ConfirmResetForm token={token} /> : <RequestResetForm />}
    </div>
  );
}

export default function ResetPasswordPage() {
  return (
    <Suspense fallback={<Cargando />}>
      <ResetPasswordInner />
    </Suspense>
  );
}

function Cargando() {
  const { t } = useI18n();
  return <p>{t("comun.cargando")}</p>;
}
