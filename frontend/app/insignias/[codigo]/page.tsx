"use client";

import { useEffect, useState } from "react";
import { useParams } from "next/navigation";
import { api, ApiError, type VerificacionInsignia } from "@/lib/api";

/**
 * Verificación pública de una insignia.
 *
 * No exige sesión: ese es el punto de una insignia verificable. Y no muestra
 * quién la tiene —ni nombre ni correo—, porque la condición de emisión pide
 * que la URL pública no exponga datos personales. Lo que acredita es que este
 * código corresponde a una aprobación real de este curso, y si fue revocada, lo
 * dice.
 */
export default function VerificarInsigniaPage() {
  const { codigo } = useParams<{ codigo: string }>();
  const [v, setV] = useState<VerificacionInsignia | null>(null);
  const [error, setError] = useState("");

  useEffect(() => {
    api
      .verificarInsignia(codigo)
      .then(setV)
      .catch((e) => {
        if (e instanceof ApiError && e.status === 404) {
          setError("No existe ninguna insignia con este código.");
        } else {
          setError(e instanceof ApiError ? e.message : "No se pudo verificar la insignia");
        }
      });
  }, [codigo]);

  if (error) {
    return (
      <div className="columna-estrecha">
        <h1>Verificación de insignia</h1>
        <p className="error-banner" role="alert">
          {error}
        </p>
      </div>
    );
  }
  if (!v) return <p role="status">Verificando…</p>;

  return (
    <div className="columna-estrecha">
      <h1>Verificación de insignia</h1>

      {v.image_url && (
        // eslint-disable-next-line @next/next/no-img-element
        <img
          src={v.image_url}
          alt={`Insignia del curso ${v.course_title}`}
          style={{ width: "100%", maxWidth: "260px", margin: "0 auto", display: "block" }}
        />
      )}

      {v.valid ? (
        <p className="success-banner" role="status">
          Insignia válida. Acredita la aprobación del curso <strong>{v.course_title}</strong>.
        </p>
      ) : (
        <p className="error-banner" role="alert">
          Esta insignia fue revocada
          {v.revoked_at ? ` el ${new Date(v.revoked_at).toLocaleDateString()}` : ""} y ya no
          acredita nada.
        </p>
      )}

      <dl className="card">
        <dt>Curso</dt>
        <dd>{v.course_title}</dd>
        <dt>Emitida</dt>
        <dd>{new Date(v.issued_at).toLocaleDateString()}</dd>
        <dt>Código</dt>
        <dd>
          <code>{v.code}</code>
        </dd>
      </dl>

      <p className="muted">
        Esta página no revela quién obtuvo la insignia. Verifica que el código
        corresponde a una aprobación real, no la identidad de nadie.
      </p>
    </div>
  );
}
