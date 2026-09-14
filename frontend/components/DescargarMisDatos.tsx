"use client";

import { useState } from "react";

import { useI18n } from "@/lib/i18n";

const API_URL = process.env.NEXT_PUBLIC_API_URL ?? "http://localhost:8080";

/**
 * Descarga de los datos personales (alcance opcional 5.2).
 *
 * El archivo se pide con fetch y se guarda desde un blob en lugar de abrir la
 * URL directamente: la API está en otro origen y exige la cookie de sesión,
 * que un enlace normal no envía con credentials.
 *
 * El objeto de la URL temporal se libera al terminar. Sin eso, cada descarga
 * deja el archivo entero retenido en memoria hasta recargar la página.
 */
export function DescargarMisDatos() {
  const { t } = useI18n();
  const [estado, setEstado] = useState<"inicial" | "descargando" | "error">("inicial");

  async function descargar() {
    setEstado("descargando");
    try {
      const res = await fetch(`${API_URL}/api/v1/me/export`, { credentials: "include" });
      if (!res.ok) throw new Error(String(res.status));

      const blob = await res.blob();
      const url = URL.createObjectURL(blob);
      const enlace = document.createElement("a");
      enlace.href = url;
      enlace.download = `mis-datos-${new Date().toISOString().slice(0, 10)}.json`;
      document.body.appendChild(enlace);
      enlace.click();
      enlace.remove();
      URL.revokeObjectURL(url);
      setEstado("inicial");
    } catch {
      setEstado("error");
    }
  }

  return (
    <section className="card stack">
      <header>
        <h2>{t("cuenta.exportar")}</h2>
        <p>{t("cuenta.exportarDescripcion")}</p>
      </header>
      {estado === "error" && (
        <p className="error-banner" role="alert">
          {t("cuenta.exportarError")}
        </p>
      )}
      <div>
        <button onClick={descargar} disabled={estado === "descargando"}>
          {estado === "descargando" ? t("cuenta.exportando") : t("cuenta.exportar")}
        </button>
      </div>
    </section>
  );
}
