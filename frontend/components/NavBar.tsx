"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { useAuth } from "@/lib/auth-context";
import { IDIOMAS, NOMBRES_DE_IDIOMA, useI18n, type Idioma } from "@/lib/i18n";

export function NavBar() {
  const { user, loading, logout } = useAuth();
  const { t, idioma, cambiar } = useI18n();
  const router = useRouter();

  async function handleLogout() {
    await logout();
    router.push("/");
  }

  return (
    <header className="navbar">
      <Link href="/" className="brand">
        Plataforma MOOC
      </Link>
      <nav aria-label={t("nav.principal")}>
        <Link href="/">{t("nav.catalogo")}</Link>
        {!loading && user?.role === "student" && <Link href="/mis-cursos">{t("nav.misCursos")}</Link>}
        {!loading && user?.role === "student" && (
          <Link href="/mis-insignias">{t("nav.misInsignias")}</Link>
        )}
        {!loading && (user?.role === "teacher" || user?.role === "admin") && (
          <Link href="/profesor">{t("nav.autoria")}</Link>
        )}
        {!loading && user?.role === "admin" && <Link href="/admin">{t("nav.administracion")}</Link>}

        {!loading && user && (
          <span className="row">
            <span className="badge">{user.full_name} · {user.role}</span>
            <Link href="/cuenta/sesiones">{t("nav.sesiones")}</Link>
            <button className="secondary" onClick={handleLogout}>
              {t("nav.salir")}
            </button>
          </span>
        )}
        {!loading && !user && (
          <span className="row">
            <Link href="/login">{t("nav.entrar")}</Link>
            <Link href="/registro">{t("nav.registrarse")}</Link>
          </span>
        )}

        <span className="row">
          <label htmlFor="idioma" className="visually-hidden">
            {t("nav.idioma")}
          </label>
          <select
            id="idioma"
            value={idioma}
            onChange={(e) => cambiar(e.target.value as Idioma)}
            style={{ padding: "2px 6px", fontSize: "0.85rem" }}
          >
            {IDIOMAS.map((i) => (
              <option key={i} value={i}>
                {NOMBRES_DE_IDIOMA[i]}
              </option>
            ))}
          </select>
        </span>
      </nav>
    </header>
  );
}
