"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import Link from "next/link";
import { api, ApiError } from "@/lib/api";
import { useAuth } from "@/lib/auth-context";

export default function LoginPage() {
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const router = useRouter();
  const { refresh } = useAuth();

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
      setError(e instanceof ApiError ? e.message : "No se pudo iniciar sesión");
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <div style={{ maxWidth: 420 }}>
      <h1>Iniciar sesión</h1>
      {error && (
        <p className="error-banner" role="alert">
          {error}
        </p>
      )}
      <form onSubmit={handleSubmit} className="stack">
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
            autoComplete="current-password"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
          />
        </div>
        <button type="submit" disabled={submitting}>
          {submitting ? "Ingresando…" : "Ingresar"}
        </button>
      </form>
      <p>
        <Link href="/restablecer-contrasena">¿Olvidaste tu contraseña?</Link>
      </p>
      <p>
        ¿No tienes cuenta? <Link href="/registro">Regístrate</Link>
      </p>
    </div>
  );
}
