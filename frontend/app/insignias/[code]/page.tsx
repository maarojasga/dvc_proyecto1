"use client";

import { useEffect, useState } from "react";
import { useParams } from "next/navigation";
import { api, ApiError, type BadgeVerification } from "@/lib/api";
import { useI18n } from "@/lib/i18n";

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
  const { t, locale } = useI18n();
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
            ? t("verificacion.noExiste")
            : t("verificacion.error"),
        );
      });
    return () => {
      cancelado = true;
    };
  }, [code, t]);

  return (
    <div className="columna-estrecha">
      <header className="page-header">
        <h1>{t("verificacion.titulo")}</h1>
      </header>

      {error && (
        <p className="error-banner" role="alert">
          {error}
        </p>
      )}

      {!error && !insignia && <p role="status">{t("verificacion.enCurso")}</p>}

      {insignia && (
        <div className="card">
          {insignia.image_url && (
            /* eslint-disable-next-line @next/next/no-img-element */
            <img
              src={insignia.image_url}
              alt={courseTitle ? t("verificacion.alt", { curso: courseTitle }) : t("verificacion.altSinCurso")}
              width={200}
              height={200}
              style={{ display: "block", margin: "0 auto 1rem" }}
            />
          )}
          {insignia.valid ? (
            <p className="success-banner" role="status">
              {t("verificacion.valida")}
            </p>
          ) : (
            <p className="warning-banner" role="status">
              {t("verificacion.revocada")}
            </p>
          )}
          <dl className="stack">
            <div>
              <dt className="muted">{t("verificacion.codigo")}</dt>
              <dd>
                <code>{insignia.code}</code>
              </dd>
            </div>
            <div>
              <dt className="muted">{t("verificacion.curso")}</dt>
              <dd>{courseTitle ?? insignia.course_id}</dd>
            </div>
            <div>
              <dt className="muted">{t("verificacion.emitida")}</dt>
              <dd>{new Date(insignia.issued_at).toLocaleDateString(locale)}</dd>
            </div>
            {insignia.revoked_at && (
              <div>
                <dt className="muted">{t("verificacion.revocadaEl")}</dt>
                <dd>{new Date(insignia.revoked_at).toLocaleDateString(locale)}</dd>
              </div>
            )}
          </dl>

          {/* Open Badges 3.0: la credencial es portátil, se verifica con la
              clave pública del emisor sin volver a llamar a esta plataforma.
              Solo se ofrece mientras la insignia siga vigente: una credencial
              firmada no se puede desdecir cuando alguien revoca. */}
          {insignia.valid && (
            <p className="row">
              <a href={api.openBadgeURL(insignia.code)} target="_blank" rel="noopener noreferrer">
                {t("verificacion.credencial")}
              </a>
              <a href={api.openBadgeJWTURL(insignia.code)} target="_blank" rel="noopener noreferrer">
                {t("verificacion.firmada")}
              </a>
            </p>
          )}
        </div>
      )}
    </div>
  );
}
