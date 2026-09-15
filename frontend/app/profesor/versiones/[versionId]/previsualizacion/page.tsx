"use client";

import { useCallback, useEffect, useState } from "react";
import Link from "next/link";
import { useParams } from "next/navigation";
import { api, ApiError, type Version } from "@/lib/api";
import { useI18n } from "@/lib/i18n";

/**
 * Previsualización de autoría: el borrador tal como lo verá un estudiante.
 *
 * Solo muestra los recursos visibles y en el orden publicado, para que el
 * autor compruebe antes de publicar qué va a encontrarse quien se inscriba.
 * Los recursos ocultos y los que aún se están procesando se señalan aparte,
 * porque son los que impiden publicar.
 */
export default function PrevisualizacionPage() {
  const params = useParams<{ versionId: string }>();
  const versionId = params.versionId;
  const { t } = useI18n();
  const [version, setVersion] = useState<Version | null>(null);
  const [error, setError] = useState("");

  const cargar = useCallback(async () => {
    try {
      setVersion(await api.getVersion(versionId));
    } catch (e) {
      setError(e instanceof ApiError ? e.message : t("previsualizacion.error"));
    }
  }, [versionId, t]);

  useEffect(() => {
    void cargar();
  }, [cargar]);

  if (error) {
    return <p className="error-banner" role="alert">{error}</p>;
  }
  if (!version) {
    return <p role="status">{t("previsualizacion.cargando")}</p>;
  }

  const modulos = [...(version.Modules ?? [])].sort((a, b) => a.Position - b.Position);
  const ocultos = modulos.flatMap((m) =>
    (m.Units ?? []).flatMap((u) => (u.Resources ?? []).filter((r) => !r.Visible)),
  );
  const enProceso = modulos.flatMap((m) =>
    (m.Units ?? []).flatMap((u) =>
      (u.Resources ?? []).filter((r) => r.ProcessingStatus === "pending" || r.ProcessingStatus === "processing"),
    ),
  );

  return (
    <div>
      <div className="row" style={{ justifyContent: "space-between", alignItems: "baseline" }}>
        <h1>{t("previsualizacion.titulo")}</h1>
        <Link href={`/profesor/versiones/${versionId}`}>{t("previsualizacion.volver")}</Link>
      </div>

      <p className="muted">
        {t("previsualizacion.intro", { n: version.VersionNumber })}{" "}
        <span className="badge">{version.Status}</span>
      </p>

      {enProceso.length > 0 && (
        <p className="warning-banner" role="status">
          {t("previsualizacion.enProceso", { n: enProceso.length })}
        </p>
      )}
      {ocultos.length > 0 && (
        <p className="muted">{t("previsualizacion.ocultos", { n: ocultos.length })}</p>
      )}

      <section className="card">
        <h2 style={{ marginTop: 0 }}>{version.Title || t("previsualizacion.sinTitulo")}</h2>
        {version.Summary && <p>{version.Summary}</p>}
        <p className="muted">
          {t("previsualizacion.aprobacion", {
            nota: version.ApprovalMinScore,
            pct: version.ApprovalRequiredResourcesPct,
          })}
        </p>
      </section>

      {modulos.length === 0 && <p className="muted">{t("previsualizacion.sinModulos")}</p>}

      {modulos.map((m) => (
        <section key={m.ID} className="card">
          <h2 style={{ marginTop: 0 }}>{m.Title}</h2>
          {[...(m.Units ?? [])]
            .sort((a, b) => a.Position - b.Position)
            .map((u) => {
              const visibles = (u.Resources ?? [])
                .filter((r) => r.Visible)
                .sort((a, b) => a.Position - b.Position);
              return (
                <div key={u.ID} className="stack">
                  <h3>{u.Title}</h3>
                  {visibles.length === 0 ? (
                    <p className="muted">{t("previsualizacion.sinVisibles")}</p>
                  ) : (
                    <ul>
                      {visibles.map((r) => (
                        <li key={r.ID}>
                          {r.Title} <span className="badge">{r.Type}</span>
                          {r.Required && <span className="badge">{t("curso.obligatorio")}</span>}
                        </li>
                      ))}
                    </ul>
                  )}
                </div>
              );
            })}
        </section>
      ))}
    </div>
  );
}
