"use client";

import { useEffect, useState } from "react";
import { useParams } from "next/navigation";
import { api, type Version, ApiError } from "@/lib/api";
import { useAuth } from "@/lib/auth-context";

export default function CourseDetailPage() {
  const { courseId } = useParams<{ courseId: string }>();
  const { user } = useAuth();
  const [version, setVersion] = useState<Version | null>(null);
  const [error, setError] = useState("");
  const [enrollMessage, setEnrollMessage] = useState("");

  useEffect(() => {
    api
      .getPublishedCourse(courseId)
      .then(setVersion)
      .catch((e) => setError(e instanceof ApiError ? e.message : "No se pudo cargar el curso"));
  }, [courseId]);

  async function handleEnroll() {
    setEnrollMessage("");
    try {
      await api.enroll(courseId);
      setEnrollMessage("¡Inscripción realizada! Consulta 'Mis cursos'.");
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
      <h1>{version.Title}</h1>
      <p>{version.Summary}</p>
      <div className="row">
        <span className="badge">{version.Category}</span>
        <span className="badge">{version.Level}</span>
        <span className="badge">{version.Language}</span>
      </div>

      {user?.role === "student" && (
        <div className="card">
          <button onClick={handleEnroll}>Inscribirme</button>
          {enrollMessage && <p role="status">{enrollMessage}</p>}
        </div>
      )}

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
                        {r.Title} <span className="badge">{r.Type}</span>
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
