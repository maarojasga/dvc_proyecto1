"use client";

import { useState } from "react";
import Link from "next/link";
import { api, ApiError } from "@/lib/api";
import { useI18n } from "@/lib/i18n";

export default function RegisterPage() {
  const [fullName, setFullName] = useState("");
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState("");
  const [done, setDone] = useState(false);
  const [submitting, setSubmitting] = useState(false);
  const { t } = useI18n();

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    setError("");
    setSubmitting(true);
    try {
      await api.register({ email, password, full_name: fullName });
      setDone(true);
    } catch (e) {
      setError(e instanceof ApiError ? [e.message, ...(e.details ?? [])].join(" ") : t("auth.registro.error"));
    } finally {
      setSubmitting(false);
    }
  }

  if (done) {
    return (
      <div className="columna-estrecha">
        <h1>{t("auth.registro.revisaTitulo")}</h1>
        <p className="success-banner">{t("auth.registro.revisaTexto", { email })}</p>
        <Link href="/login">{t("auth.irALogin")}</Link>
      </div>
    );
  }

  return (
    <div className="columna-estrecha">
      <h1>{t("auth.registro.titulo")}</h1>
      <p className="muted">{t("auth.registro.subtitulo")}</p>
      {error && (
        <p className="error-banner" role="alert">
          {error}
        </p>
      )}
      <form onSubmit={handleSubmit} className="stack">
        <div className="form-field">
          <label htmlFor="full_name">{t("auth.nombreCompleto")}</label>
          <input id="full_name" required value={fullName} onChange={(e) => setFullName(e.target.value)} />
        </div>
        <div className="form-field">
          <label htmlFor="email">{t("auth.correo")}</label>
          <input
            id="email"
            type="email"
            required
            autoComplete="email"
            value={email}
            onChange={(e) => setEmail(e.target.value)}
          />
        </div>
        <div className="form-field">
          <label htmlFor="password">{t("auth.contrasena")}</label>
          <input
            id="password"
            type="password"
            required
            minLength={10}
            autoComplete="new-password"
            aria-describedby="password-help"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
          />
          <span id="password-help" className="badge">{t("auth.registro.minimo")}</span>
        </div>
        <button type="submit" disabled={submitting}>
          {submitting ? t("auth.registro.enviando") : t("auth.registro.enviar")}
        </button>
      </form>
      <p>
        {t("auth.registro.yaTienes")} <Link href="/login">{t("auth.registro.iniciaSesion")}</Link>
      </p>
    </div>
  );
}
