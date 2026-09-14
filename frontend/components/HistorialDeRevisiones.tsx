"use client";

import { useCallback, useEffect, useState } from "react";

import { api, ApiError, type Revision } from "@/lib/api";

/**
 * Historial visible de revisiones de un recurso.
 *
 * El autoguardado del editor conserva el borrador en el navegador, que se
 * pierde al cambiar de equipo y no dice quién escribió qué. Esto es lo que lo
 * hace un historial de verdad: cada guardado deja una revisión numerada,
 * atribuida y restaurable desde el servidor.
 *
 * Restaurar no retrocede el historial: deja una revisión nueva con el
 * contenido antiguo, así que deshacer una restauración equivocada es otra
 * restauración y no una pérdida.
 */
export function HistorialDeRevisiones({
  versionId,
  resourceId,
  onRestaurado,
}: {
  versionId: string;
  resourceId: string;
  /** Se llama tras restaurar, para que el editor recargue el contenido. */
  onRestaurado: () => void;
}) {
  const [items, setItems] = useState<Revision[] | null>(null);
  const [abierta, setAbierta] = useState<Revision | null>(null);
  const [error, setError] = useState("");
  const [ocupado, setOcupado] = useState(false);

  const cargar = useCallback(async () => {
    try {
      const res = await api.listRevisions(versionId, resourceId);
      setItems(res.items ?? []);
    } catch (e) {
      setError(e instanceof ApiError ? e.message : "No se pudo cargar el historial");
    }
  }, [versionId, resourceId]);

  useEffect(() => {
    cargar();
  }, [cargar]);

  async function ver(numero: number) {
    setError("");
    try {
      setAbierta(await api.getRevision(versionId, resourceId, numero));
    } catch (e) {
      setError(e instanceof ApiError ? e.message : "No se pudo abrir la revisión");
    }
  }

  async function restaurar(numero: number) {
    if (!confirm(`¿Restaurar la revisión ${numero}? Se guardará como una revisión nueva.`)) return;
    setOcupado(true);
    setError("");
    try {
      await api.restoreRevision(versionId, resourceId, numero);
      setAbierta(null);
      await cargar();
      onRestaurado();
    } catch (e) {
      setError(e instanceof ApiError ? e.message : "No se pudo restaurar");
    } finally {
      setOcupado(false);
    }
  }

  if (items === null) return null;

  return (
    <section className="card stack">
      <header>
        <h3 style={{ margin: 0 }}>Historial de revisiones</h3>
      </header>

      {error && (
        <p className="error-banner" role="alert">
          {error}
        </p>
      )}

      {items.length === 0 ? (
        <p className="muted">Todavía no hay revisiones guardadas de este recurso.</p>
      ) : (
        <ul className="lista-filas">
          {items.map((rev) => (
            <li key={rev.id} className="fila">
              <div className="fila__datos">
                <p>
                  <span className="badge">#{rev.revision_number}</span>{" "}
                  {new Date(rev.created_at).toLocaleString("es-CO")}
                </p>
                {rev.author_email && <p className="muted">Guardada por {rev.author_email}</p>}
              </div>
              <button className="secondary" onClick={() => ver(rev.revision_number)}>
                Ver
              </button>
              <button
                className="secondary"
                disabled={ocupado}
                onClick={() => restaurar(rev.revision_number)}
              >
                Restaurar
              </button>
            </li>
          ))}
        </ul>
      )}

      {abierta && (
        <div className="card stack">
          <div className="row" style={{ justifyContent: "space-between", alignItems: "baseline" }}>
            <strong>Revisión #{abierta.revision_number}</strong>
            <button className="secondary" onClick={() => setAbierta(null)}>
              Cerrar
            </button>
          </div>
          <pre
            style={{
              whiteSpace: "pre-wrap",
              wordBreak: "break-word",
              maxHeight: "20rem",
              overflow: "auto",
              margin: 0,
            }}
          >
            {abierta.content_md}
          </pre>
        </div>
      )}
    </section>
  );
}
