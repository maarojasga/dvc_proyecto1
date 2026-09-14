"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import Link from "next/link";
import { api, ApiError } from "@/lib/api";
import { useAuth } from "@/lib/auth-context";
import { useI18n } from "@/lib/i18n";

export default function LoginPage() {
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [showPassword, setShowPassword] = useState(false);
  const [error, setError] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const router = useRouter();
  const { refresh } = useAuth();
  const { t } = useI18n();

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    setError("");
    setSubmitting(true);
    try {
      const { user } = await api.login(email, password);
      await refresh();
      if (user.role === "admin") router.push("/admin");
      else if (user.role === "teacher") router.push("/profesor");
      else router.push("/mis-cursos");
    } catch (e) {
      setError(e instanceof ApiError ? e.message : t("auth.login.error"));
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <div className="columna-estrecha">
      <h1>{t("auth.login.titulo")}</h1>
      <p className="muted">{t("auth.login.subtitulo")}</p>
      {error && (
        <p className="error-banner" role="alert">
          {error}
        </p>
      )}
      <form onSubmit={handleSubmit} className="stack">
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
          <div className="row" style={{ justifyContent: "space-between", alignItems: "center", marginBottom: "0.25rem" }}>
            <label htmlFor="password" style={{ marginBottom: 0 }}>{t("auth.contrasena")}</label>
            <button
              type="button"
              onClick={() => setShowPassword(!showPassword)}
              style={{ background: "transparent", border: "none", color: "#3b82f6", cursor: "pointer", fontSize: "0.8rem", padding: "0 4px", textDecoration: "underline" }}
            >
              {showPassword ? t("auth.ocultar") : t("auth.mostrar")}
            </button>
          </div>
          <input
            id="password"
            type={showPassword ? "text" : "password"}
            required
            autoComplete="current-password"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
          />
        </div>
        <button type="submit" disabled={submitting}>
          {submitting ? t("auth.login.enviando") : t("auth.login.enviar")}
        </button>
      </form>
      <p>
        <Link href="/restablecer-contrasena">{t("auth.login.olvide")}</Link>
      </p>
      <p>
        {t("auth.login.sinCuenta")} <Link href="/registro">{t("auth.login.registrate")}</Link>
      </p>
    </div>
  );
}
