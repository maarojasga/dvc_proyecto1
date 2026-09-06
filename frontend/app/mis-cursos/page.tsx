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
      <p>
        Debes <Link href="/login">iniciar sesión</Link> para ver tus cursos.
      </p>
    );
  }

  return (
    <div>
      <h1>Mis cursos</h1>
      {error && (
        <p className="error-banner" role="alert">
          {error}
        </p>
      )}
      {items === null && <p>Cargando…</p>}
      {items?.length === 0 && <p>Aún no te has inscrito a ningún curso. Explora el catálogo.</p>}
      <ul className="stack" style={{ listStyle: "none", padding: 0 }}>
        {items?.map((e) => (
          <li key={e.ID} className="card row">
            <div style={{ flex: 1 }}>
              <Link href={`/cursos/${e.CourseID}`}>Ver curso</Link>
              <p className="badge">{statusLabel[e.Status]}</p>
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
