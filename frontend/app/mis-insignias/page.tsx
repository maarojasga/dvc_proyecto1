"use client";

import { useEffect, useState } from "react";
import Link from "next/link";
import { api, type Badge, ApiError } from "@/lib/api";
import { useAuth } from "@/lib/auth-context";
import { useI18n } from "@/lib/i18n";

export default function MyBadgesPage() {
  const { user, loading: authLoading } = useAuth();
  const [items, setItems] = useState<Badge[] | null>(null);
  const [error, setError] = useState("");
  const { t, locale } = useI18n();

  useEffect(() => {
    if (authLoading || !user) return;
    api
      .listMyBadges()
      .then((res) => setItems(res.items ?? []))
      .catch((e) => setError(e instanceof ApiError ? e.message : t("insignias.error")));
  }, [authLoading, user, t]);

  if (!authLoading && !user) {
    return (
      <div className="estado-vacio columna-estrecha">
        <p>{t("insignias.requiereSesion")}</p>
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
          <h1>{t("nav.misInsignias")}</h1>
          <p>{t("insignias.subtitulo")}</p>
        </div>
      </header>

      {error && (
        <p className="error-banner" role="alert">
          {error}
        </p>
      )}
      {items === null && !error && <p role="status">{t("comun.cargando")}</p>}

      {items?.length === 0 && (
        <div className="estado-vacio">
          <p>{t("insignias.vacio")}</p>
          <p>{t("insignias.vacioAccion")}</p>
        </div>
      )}

      <ul className="lista-filas">
        {items?.map((b) => (
          <li key={b.code} className="card fila">
            {b.image_url && (
              /* eslint-disable-next-line @next/next/no-img-element */
              <img
                src={b.image_url}
                alt={t("insignias.alt", { fecha: new Date(b.issued_at).toLocaleDateString(locale) })}
                width={96}
                height={96}
                style={{ borderRadius: "8px" }}
              />
            )}
            <div className="fila__datos">
              <p>
                <span className="badge">
                  {b.valid ? t("insignias.vigente") : t("insignias.revocada")}
                </span>{" "}
                {t("insignias.emitidaEl", { fecha: new Date(b.issued_at).toLocaleDateString(locale) })}
              </p>
              <p className="muted">
                {t("insignias.codigoVerificacion")} <code>{b.code}</code>
              </p>
            </div>
            <Link href={`/insignias/${b.code}`}>{t("curso.verVerificacion")}</Link>
          </li>
        ))}
      </ul>
    </div>
  );
}
