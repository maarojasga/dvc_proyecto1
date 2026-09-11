"use client";

import { useEffect, useState } from "react";
import Link from "next/link";
import { api, ApiError, type InsigniaPropia } from "@/lib/api";

/**
 * Las insignias del estudiante.
 *
 * Cada una lleva su enlace de verificación, que es lo que se comparte: la
 * página de verificación es pública y no expone datos personales, así que se
 * puede pegar en un currículum sin filtrar el correo de nadie.
 */
export default function MisInsigniasPage() {
  const [insignias, setInsignias] = useState<InsigniaPropia[] | null>(null);
  const [error, setError] = useState("");

  useEffect(() => {
    api
      .misInsignias()
      .then((r) => setInsignias(r.items ?? []))
      .catch((e) => setError(e instanceof ApiError ? e.message : "No se pudieron cargar las insignias"));
  }, []);

  if (error) {
    return (
      <p className="error-banner" role="alert">
        {error}
      </p>
    );
  }
  if (!insignias) return <p role="status">Cargando insignias…</p>;

  return (
    <div>
      <header className="page-header">
        <div>
          <h1>Mis insignias</h1>
          <p className="muted">Se emite una al aprobar un curso, y su enlace de verificación es público.</p>
        </div>
      </header>

      {insignias.length === 0 ? (
        <div className="estado-vacio">
          <p>Todavía no tienes insignias.</p>
          <p>
            Se emiten al aprobar un curso: completa los recursos obligatorios y sus
            evaluaciones. <Link href="/mis-cursos">Ver mis cursos</Link>
          </p>
        </div>
      ) : (
        <ul className="rejilla">
          {insignias.map((b) => (
            <li key={b.code} className="card">
              {b.image_url && (
                // La imagen la genera el servidor al emitir la insignia y vive
                // en el almacén de objetos, no en la base de datos. La URL es
                // firmada o de CDN según el entorno, así que no puede
                // declararse en tiempo de compilación para next/image.
                // eslint-disable-next-line @next/next/no-img-element
                <img
                  src={b.image_url}
                  alt={`Insignia del curso ${b.course_title}`}
                  style={{ width: "100%", maxWidth: "220px", margin: "0 auto", display: "block" }}
                />
              )}
              <h2>{b.course_title}</h2>
              <p className="muted">Emitida el {new Date(b.issued_at).toLocaleDateString()}</p>
              {b.valid ? (
                <p>
                  <Link href={`/insignias/${encodeURIComponent(b.code)}`}>Enlace de verificación</Link>
                </p>
              ) : (
                <p className="warning-banner" role="status">
                  Revocada{b.revoked_reason ? `: ${b.revoked_reason}` : ""}
                </p>
              )}
              <p className="muted">
                <code>{b.code}</code>
              </p>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}
