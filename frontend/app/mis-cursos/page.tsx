"use client";

import { useEffect, useState } from "react";
import Link from "next/link";
import { api, type Enrollment, ApiError } from "@/lib/api";
import { useAuth } from "@/lib/auth-context";
import { useI18n, type Clave } from "@/lib/i18n";

// El estado de la inscripción llega como identificador del dominio; la clave
// del catálogo se deriva de él para que un estado nuevo en la API falle al
// compilar en vez de aparecer sin traducir.
const claveDeEstado: Record<Enrollment["Status"], Clave> = {
  active: "estado.active",
  withdrawn: "estado.withdrawn",
  completed: "estado.completed",
  approved: "estado.approved",
};

export default function MyCoursesPage() {
  const { user, loading: authLoading } = useAuth();
  const [items, setItems] = useState<Enrollment[] | null>(null);
  const [error, setError] = useState("");
  const { t } = useI18n();

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
      setError(e instanceof ApiError ? e.message : t("misCursos.error"));
    }
  }

  async function handleWithdraw(courseId: string) {
    try {
      await api.withdraw(courseId);
      load();
    } catch (e) {
      setError(e instanceof ApiError ? e.message : t("misCursos.errorRetirar"));
    }
  }

  if (!authLoading && !user) {
    return (
      <div className="estado-vacio columna-estrecha">
        <p>{t("misCursos.requiereSesion")}</p>
        <p>
          <Link href="/login">{t("nav.entrar")}</Link>
        </p>
      </div>
    );
  }

  return (
    <div>
      <header className="page-header">
        <div>
          <h1>{t("nav.misCursos")}</h1>
          <p>{t("misCursos.subtitulo")}</p>
        </div>
      </header>

      {error && (
        <p className="error-banner" role="alert">
          {error}
        </p>
      )}
      {items === null && <p role="status">{t("comun.cargando")}</p>}

      {items?.length === 0 && (
        <div className="estado-vacio">
          <p>{t("misCursos.vacio")}</p>
          <p>
            <Link href="/">{t("misCursos.explorar")}</Link>
          </p>
        </div>
      )}

      <ul className="lista-filas">
        {items?.map((e) => (
          <li key={e.ID} className="card fila">
            <div className="fila__datos">
              <Link href={`/cursos/${e.CourseID}`}>{t("misCursos.verCurso")}</Link>
              <p>
                <span className="badge">{t(claveDeEstado[e.Status])}</span>
              </p>
            </div>
            {e.Status === "active" && (
              <button className="secondary" onClick={() => handleWithdraw(e.CourseID)}>
                {t("misCursos.retirarme")}
              </button>
            )}
          </li>
        ))}
      </ul>
    </div>
  );
}
