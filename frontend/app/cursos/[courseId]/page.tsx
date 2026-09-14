"use client";

import { useCallback, useEffect, useState } from "react";
import Link from "next/link";
import { useParams } from "next/navigation";
import { api, type Version, type CourseProgress, ApiError } from "@/lib/api";
import { useAuth } from "@/lib/auth-context";
import { useI18n, type Clave } from "@/lib/i18n";

const claveDeEstado: Record<CourseProgress["status"], Clave> = {
  active: "estado.active",
  withdrawn: "estado.withdrawn",
  completed: "estado.completed",
  approved: "estado.approved",
};

export default function CourseDetailPage() {
  const { courseId } = useParams<{ courseId: string }>();
  const { user } = useAuth();
  const { t } = useI18n();
  const [version, setVersion] = useState<Version | null>(null);
  const [error, setError] = useState("");
  const [enrollMessage, setEnrollMessage] = useState("");
  const [progress, setProgress] = useState<CourseProgress | null>(null);

  useEffect(() => {
    api
      .getPublishedCourse(courseId)
      .then(setVersion)
      .catch((e) => setError(e instanceof ApiError ? e.message : t("curso.error")));
  }, [courseId, t]);

  const loadProgress = useCallback(() => {
    if (user?.role !== "student") return;
    // Un 404 aquí significa "no inscrito", que es un estado normal y no un
    // error: por eso se ignora en silencio en vez de mostrar un banner.
    api.getCourseProgress(courseId).then(setProgress).catch(() => setProgress(null));
  }, [courseId, user?.role]);

  useEffect(() => {
    loadProgress();
  }, [loadProgress]);

  async function handleEnroll() {
    setEnrollMessage("");
    try {
      await api.enroll(courseId);
      setEnrollMessage(t("curso.inscrito"));
      loadProgress();
    } catch (e) {
      setEnrollMessage(e instanceof ApiError ? e.message : t("curso.errorInscripcion"));
    }
  }

  if (error) {
    return (
      <p className="error-banner" role="alert">
        {error}
      </p>
    );
  }
  if (!version) {
    return <p>{t("curso.cargando")}</p>;
  }

  return (
    <div>
      <header className="page-header">
        <div>
          <h1>{version.Title}</h1>
          <p>{version.Summary}</p>
          <div className="row" style={{ marginTop: "0.5rem" }}>
            {version.Category && <span className="badge">{version.Category}</span>}
            {version.Level && <span className="badge">{version.Level}</span>}
            <span className="badge">{version.Language}</span>
          </div>
        </div>
        {user?.role === "student" && !progress && (
          <div>
            <button onClick={handleEnroll}>{t("curso.inscribirme")}</button>
          </div>
        )}
      </header>

      {enrollMessage && (
        <p className="success-banner" role="status">
          {enrollMessage}
        </p>
      )}

      {progress && (
        <div className="card">
          <h2 style={{ marginTop: 0 }}>{t("curso.tuAvance")}</h2>
          <p className="row" style={{ alignItems: "center" }}>
            <span className="badge">{t(claveDeEstado[progress.status])}</span>
            <span>
              {t("curso.recursosCompletados", {
                hechos: progress.required_completed,
                total: progress.required_total,
                pct: progress.required_percent.toFixed(0),
              })}
            </span>
          </p>
          {progress.quizzes_pending > 0 && (
            <p className="muted">
              {t("curso.evaluacionesPendientes", { n: progress.quizzes_pending })}
            </p>
          )}
          {progress.badge_code && (
            <p className="success-banner" role="status">
              {t("curso.insigniaObtenida")}{" "}
              <Link href={`/insignias/${progress.badge_code}`}>{t("curso.verVerificacion")}</Link>
            </p>
          )}
        </div>
      )}

      <h2>{t("curso.contenido")}</h2>
      <ol className="stack">
        {version.Modules?.map((m) => (
          <li key={m.ID} className="card">
            <h3>{m.Title}</h3>
            <ol>
              {m.Units?.map((u) => (
                <li key={u.ID}>
                  <strong>{u.Title}</strong>
                  <ul>
                    {u.Resources?.filter((r) => r.Visible).map((r) => (
                      <li key={r.ID}>
                        {/* Solo quien esté inscrito recibirá el contenido; la
                            API lo comprueba en cada entrega. El enlace se
                            muestra igual para que se vea qué trae el curso. */}
                        <Link href={`/cursos/${courseId}/recursos/${r.ID}`}>{r.Title}</Link>{" "}
                        <span className="badge">{r.Type}</span>
                        {r.Required && <span className="badge">{t("curso.obligatorio")}</span>}
                      </li>
                    ))}
                  </ul>
                </li>
              ))}
            </ol>
          </li>
        ))}
      </ol>
    </div>
  );
}
