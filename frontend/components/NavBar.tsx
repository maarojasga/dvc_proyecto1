"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { useAuth } from "@/lib/auth-context";

export function NavBar() {
  const { user, loading, logout } = useAuth();
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
      <nav aria-label="Principal">
        <Link href="/">Catálogo</Link>
        {!loading && user?.role === "student" && <Link href="/mis-cursos">Mis cursos</Link>}
        {!loading && (user?.role === "teacher" || user?.role === "admin") && (
          <Link href="/profesor">Autoría</Link>
        )}
        {!loading && user?.role === "admin" && <Link href="/admin">Administración</Link>}

        {!loading && user && (
          <span className="row">
            <span className="badge">{user.full_name} · {user.role}</span>
            <button className="secondary" onClick={handleLogout}>
              Cerrar sesión
            </button>
          </span>
        )}
        {!loading && !user && (
          <span className="row">
            <Link href="/login">Iniciar sesión</Link>
            <Link href="/registro">Registrarse</Link>
          </span>
        )}
      </nav>
    </header>
  );
}
