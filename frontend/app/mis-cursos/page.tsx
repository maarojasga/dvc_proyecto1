"use client";

import { useEffect, useState } from "react";
import Link from "next/link";
import { api, type Enrollment, ApiError } from "@/lib/api";
import { useAuth } from "@/lib/auth-context";

const statusLabel: Record<Enrollment["Status"], string> = {
  active: "En curso",
  withdrawn: "Retirado",
  completed: "Completado",
  approved: "Aprobado",
};

export default function MyCoursesPage() {
  const { user, loading: authLoading } = useAuth();
  const [items, setItems] = useState<Enrollment[] | null>(null);
  const [error, setError] = useState("");

  useEffect(() => {
    if (!authLoading && user) {
      load();
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [authLoading, user]);

  async function load() {
    try {
      const res = await api.listMyEnrollments();
      setItems(res.items ?? []);
    } catch (e) {
      setError(e instanceof ApiError ? e.message : "No se pudieron cargar tus cursos");
    }
  }

  async function handleWithdraw(courseId: string) {
    try {
      await api.withdraw(courseId);
      load();
    } catch (e) {
      setError(e instanceof ApiError ? e.message : "No se pudo retirar la inscripción");
    }
  }

  if (!authLoading && !user) {
    return (
      <div className="estado-vacio columna-estrecha">
        <p>
          Debes <Link href="/login">iniciar sesión</Link> para ver tus cursos.
        </p>
      </div>
    );
  }

  return (
    <div>
      <header className="page-header">
        <div>
          <h1>Mis cursos</h1>
          <p>Los cursos a los que estás inscrito y su estado.</p>
        </div>
      </header>

      {error && (
        <p className="error-banner" role="alert">
          {error}
        </p>
      )}
      {items === null && <p role="status">Cargando…</p>}

      {items?.length === 0 && (
        <div className="estado-vacio">
          <p>Aún no te has inscrito a ningún curso.</p>
          <p>
            <Link href="/">Explora el catálogo</Link> para empezar.
          </p>
        </div>
      )}

      <ul className="lista-filas">
        {items?.map((e) => (
          <li key={e.ID} className="card fila">
            <div className="fila__datos">
              <Link href={`/cursos/${e.CourseID}`}>Ver curso</Link>
              <p>
                <span className="badge">{statusLabel[e.Status]}</span>
              </p>
            </div>
            {e.Status === "active" && (
              <button className="secondary" onClick={() => handleWithdraw(e.CourseID)}>
                Retirarme
              </button>
            )}
          </li>
        ))}
      </ul>
    </div>
  );
}
