"use client";

import { useEffect, useState } from "react";
import { useParams } from "next/navigation";
import { api, ApiError, type BadgeVerification } from "@/lib/api";

/**
 * El curso se resuelve aparte porque es un dato secundario: si el catálogo
 * falla o el curso ya no está publicado, la verificación de la insignia en sí
 * sigue siendo válida y no debería bloquearse por eso.
 */
function useCourseTitle(courseId: string | undefined) {
  const [title, setTitle] = useState<string | null>(null);
  useEffect(() => {
    if (!courseId) return;
    api
      .getPublishedCourse(courseId)
      .then((v) => setTitle(v.Title))
      .catch(() => setTitle(null));
  }, [courseId]);
  return title;
}

/**
 * Verificación pública de una insignia.
 *
 * No exige sesión, que es el sentido de una insignia verificable: cualquiera
 * con el enlace puede confirmar que existe y sigue vigente. Por eso no
 * expone el nombre ni el correo del estudiante, solo el curso y el estado.
 */
export default function VerificacionInsigniaPage() {
  const { code } = useParams<{ code: string }>();
  const [insignia, setInsignia] = useState<BadgeVerification | null>(null);
  const [error, setError] = useState("");
  const courseTitle = useCourseTitle(insignia?.course_id);

  useEffect(() => {
    let cancelado = false;
    api
      .verifyBadge(code)
      .then((v) => {
        if (!cancelado) setInsignia(v);
      })
      .catch((e) => {
        if (cancelado) return;
        setError(
          e instanceof ApiError && e.status === 404
            ? "No existe ninguna insignia con este código."
            : "No se pudo verificar la insignia.",
        );
      });
    return () => {
      cancelado = true;
    };
  }, [code]);

  return (
    <div className="columna-estrecha">
      <header className="page-header">
        <h1>Verificación de insignia</h1>
      </header>

      {error && (
        <p className="error-banner" role="alert">
          {error}
        </p>
      )}

      {!error && !insignia && <p role="status">Verificando…</p>}

      {insignia && (
        <div className="card">
          {insignia.image_url && (
            /* eslint-disable-next-line @next/next/no-img-element */
            <img
              src={insignia.image_url}
              alt={`Insignia del curso ${courseTitle ?? "verificada"}`}
              width={200}
              height={200}
              style={{ display: "block", margin: "0 auto 1rem" }}
            />
          )}
          {insignia.valid ? (
            <p className="success-banner" role="status">
              Esta insignia es válida.
            </p>
          ) : (
            <p className="warning-banner" role="status">
              Esta insignia fue revocada y ya no es válida.
            </p>
          )}
          <dl className="stack">
            <div>
              <dt className="muted">Código</dt>
              <dd>
                <code>{insignia.code}</code>
              </dd>
            </div>
            <div>
              <dt className="muted">Curso</dt>
              <dd>{courseTitle ?? insignia.course_id}</dd>
            </div>
            <div>
              <dt className="muted">Emitida el</dt>
              <dd>{new Date(insignia.issued_at).toLocaleDateString()}</dd>
            </div>
            {insignia.revoked_at && (
              <div>
                <dt className="muted">Revocada el</dt>
                <dd>{new Date(insignia.revoked_at).toLocaleDateString()}</dd>
              </div>
            )}
          </dl>
        </div>
      )}
    </div>
  );
}
