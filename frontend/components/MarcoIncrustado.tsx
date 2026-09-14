"use client";

import { useState } from "react";
import type { ResourceContent } from "@/lib/api";

// Presenta un recurso de tipo iframe.
//
// Los atributos de seguridad —sandbox, allow y referrerPolicy— llegan del
// servidor, que los deriva de la lista blanca. No se calculan aquí ni se
// completan con valores por defecto: si el cliente pudiera elegir cuánto
// restringir el contenido de terceros, bastaría con manipularlo para que no
// restringiera nada, y entonces la contención no valdría como control.
//
// Si el servidor no los manda, el marco no se pinta. Un iframe sin sandbox es
// exactamente lo que este recurso no debe ser.

export function MarcoIncrustado({ contenido }: { contenido: ResourceContent }) {
  const [cargado, setCargado] = useState(false);

  if (!contenido.external_url || !contenido.sandbox) {
    return (
      <p className="warning-banner" role="status">
        Este contenido incrustado no se puede mostrar porque el servidor no
        autorizó su presentación.
      </p>
    );
  }

  const host = hostDe(contenido.external_url);

  return (
    <figure className="stack" style={{ margin: 0 }}>
      <div
        style={{
          position: "relative",
          width: "100%",
          aspectRatio: "16 / 9",
          background: "var(--color-bg-subtle, #f1f5f9)",
          borderRadius: "8px",
          overflow: "hidden",
          border: "1px solid var(--color-border, #e5e7eb)",
        }}
      >
        {!cargado && (
          <p role="status" style={{ position: "absolute", inset: 0, display: "grid", placeItems: "center" }}>
            Cargando el contenido incrustado…
          </p>
        )}
        <iframe
          src={contenido.external_url}
          title={contenido.title}
          sandbox={contenido.sandbox}
          allow={contenido.allow}
          referrerPolicy={contenido.referrer_policy as React.HTMLAttributeReferrerPolicy | undefined}
          loading="lazy"
          onLoad={() => setCargado(true)}
          style={{ width: "100%", height: "100%", border: 0, display: "block" }}
        />
      </div>

      {/* Quien mira tiene derecho a saber de quién es el contenido que se le
          está sirviendo dentro de la página del curso. */}
      <figcaption className="muted">
        Contenido incrustado desde <strong>{host}</strong>.{" "}
        <a href={contenido.external_url} target="_blank" rel="noopener noreferrer">
          Abrirlo en una pestaña nueva
        </a>
      </figcaption>
    </figure>
  );
}

function hostDe(url: string): string {
  try {
    return new URL(url).hostname;
  } catch {
    return "un sitio externo";
  }
}
