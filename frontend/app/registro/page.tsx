"use client";

import { useState } from "react";
import Link from "next/link";
import { api, ApiError } from "@/lib/api";

export default function RegisterPage() {
  const [fullName, setFullName] = useState("");
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState("");
  const [done, setDone] = useState(false);
  const [submitting, setSubmitting] = useState(false);

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    setError("");
    setSubmitting(true);
    try {
      await api.register({ email, password, full_name: fullName });
      setDone(true);
    } catch (e) {
      setError(e instanceof ApiError ? [e.message, ...(e.details ?? [])].join(" ") : "No se pudo completar el registro");
    } finally {
      setSubmitting(false);
    }
  }

  if (done) {
    return (
      <div style={{ maxWidth: 480 }}>
        <h1>Revisa tu correo</h1>
        <p className="success-banner">
          Si <strong>{email}</strong> está disponible, enviamos un enlace de verificación.
          Confírmalo para poder iniciar sesión. En desarrollo, revisa Mailpit en
          http://localhost:8025.
        </p>
        <Link href="/login">Ir a iniciar sesión</Link>
      </div>
    );
  }

  return (
    <div style={{ maxWidth: 420 }}>
      <h1>Crear cuenta de estudiante</h1>
      {error && (
        <p className="error-banner" role="alert">
          {error}
        </p>
      )}
      <form onSubmit={handleSubmit} className="stack">
        <div className="form-field">
          <label htmlFor="full_name">Nombre completo</label>
          <input id="full_name" required value={fullName} onChange={(e) => setFullName(e.target.value)} />
        </div>
        <div className="form-field">
          <label htmlFor="email">Correo electrónico</label>
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
          <label htmlFor="password">Contraseña</label>
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
          <span id="password-help" className="badge">Mínimo 10 caracteres</span>
        </div>
        <button type="submit" disabled={submitting}>
          {submitting ? "Creando cuenta…" : "Registrarme"}
        </button>
      </form>
      <p>
        ¿Ya tienes cuenta? <Link href="/login">Inicia sesión</Link>
      </p>
    </div>
  );
}
