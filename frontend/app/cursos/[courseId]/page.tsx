"use client";

import { useCallback, useEffect, useState } from "react";
import Link from "next/link";
import { useParams } from "next/navigation";
import { api, type Version, type ResumenProgreso, ApiError } from "@/lib/api";
import { useAuth } from "@/lib/auth-context";

export default function CourseDetailPage() {
  const { courseId } = useParams<{ courseId: string }>();
  const { user } = useAuth();
  const [version, setVersion] = useState<Version | null>(null);
  const [progreso, setProgreso] = useState<ResumenProgreso | null>(null);
  const [error, setError] = useState("");
  const [enrollMessage, setEnrollMessage] = useState("");

  const cargarProgreso = useCallback(() => {
    if (user?.role !== "student") return;
    // Un 404 aquí solo significa "no está inscrito", que es un estado normal
    // del catálogo y no un error que mostrar.
    api
      .progresoDeCurso(courseId)
      .then(setProgreso)
      .catch(() => setProgreso(null));
  }, [courseId, user?.role]);

  useEffect(() => {
    api
      .getPublishedCourse(courseId)
      .then(setVersion)
      .catch((e) => setError(e instanceof ApiError ? e.message : "No se pudo cargar el curso"));
  }, [courseId]);

  useEffect(cargarProgreso, [cargarProgreso]);

  async function handleEnroll() {
    setEnrollMessage("");
    try {
      await api.enroll(courseId);
      setEnrollMessage("¡Inscripción realizada! Consulta 'Mis cursos'.");
      cargarProgreso();
    } catch (e) {
      setEnrollMessage(e instanceof ApiError ? e.message : "No se pudo completar la inscripción");
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
    return <p>Cargando curso…</p>;
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
        {user?.role === "student" && (
          <div>
            <button onClick={handleEnroll}>Inscribirme</button>
          </div>
        )}
      </header>

      {enrollMessage && (
        <p className="success-banner" role="status">
          {enrollMessage}
        </p>
      )}

      {progreso && <AvanceDelCurso resumen={progreso} />}

      <h2>Contenido</h2>
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
                        {r.Required && <span className="badge">obligatorio</span>}
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

/**
 * AvanceDelCurso muestra lo que el servidor calculó.
 *
 * El porcentaje no se deriva aquí de los recursos vistos: el servidor lo
 * calcula a partir de los latidos y los eventos de apertura, y rechaza
 * cualquier porcentaje que mande un cliente. Mostrar un número propio sería
 * inventar un segundo cálculo que puede discrepar del que cuenta.
 */
function AvanceDelCurso({ resumen }: { resumen: ResumenProgreso }) {
  const pct = Math.round(resumen.required_percent);
  return (
    <section className="card">
      <h2>Tu avance</h2>
      <div
        role="progressbar"
        aria-label="Avance en los recursos obligatorios"
        aria-valuenow={pct}
        aria-valuemin={0}
        aria-valuemax={100}
        style={{ width: "100%", background: "var(--color-border)", borderRadius: "999px", height: "8px", overflow: "hidden" }}
      >
        <div style={{ width: `${pct}%`, background: "var(--color-accent)", height: "100%" }} />
      </div>
      <p className="muted">
        {pct}% de los recursos obligatorios ({resumen.required_completed} de{" "}
        {resumen.required_total})
        {resumen.quizzes_pending > 0 && ` · ${resumen.quizzes_pending} evaluación(es) pendiente(s)`}
      </p>
      {resumen.status === "approved" && resumen.badge_code && (
        <p className="success-banner" role="status">
          Curso aprobado.{" "}
          <Link href={`/insignias/${encodeURIComponent(resumen.badge_code)}`}>Ver la insignia</Link>
        </p>
      )}
      {resumen.status === "completed" && (
        <p className="success-banner" role="status">
          Completaste el contenido obligatorio.
          {resumen.quizzes_pending > 0 && " Falta aprobar las evaluaciones para obtener la insignia."}
        </p>
      )}
    </section>
  );
}
