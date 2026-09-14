"use client";

import { useEffect, useState } from "react";
import Link from "next/link";
import { api, type Badge, ApiError } from "@/lib/api";
import { useAuth } from "@/lib/auth-context";

export default function MyBadgesPage() {
  const { user, loading: authLoading } = useAuth();
  const [items, setItems] = useState<Badge[] | null>(null);
  const [error, setError] = useState("");

  useEffect(() => {
    if (authLoading || !user) return;
    api
      .listMyBadges()
      .then((res) => setItems(res.items ?? []))
      .catch((e) => setError(e instanceof ApiError ? e.message : "No se pudieron cargar tus insignias"));
  }, [authLoading, user]);

  if (!authLoading && !user) {
    return (
      <div className="estado-vacio columna-estrecha">
        <p>
          Debes <Link href="/login">iniciar sesión</Link> para ver tus insignias.
        </p>
      </div>
    );
  }

  return (
    <div>
      <header className="page-header">
        <div>
          <h1>Mis insignias</h1>
          <p>Insignias verificables emitidas al aprobar un curso.</p>
        </div>
      </header>

      {error && (
        <p className="error-banner" role="alert">
          {error}
        </p>
      )}
      {items === null && !error && <p role="status">Cargando…</p>}

      {items?.length === 0 && (
        <div className="estado-vacio">
          <p>Todavía no tienes ninguna insignia.</p>
          <p>Aprueba las evaluaciones obligatorias de un curso para obtener la tuya.</p>
        </div>
      )}

      <ul className="lista-filas">
        {items?.map((b) => (
          <li key={b.ID} className="card fila">
            <div className="fila__datos">
              <p>
                {b.RevokedAt ? (
                  <span className="badge">revocada</span>
                ) : (
                  <span className="badge">vigente</span>
                )}{" "}
                Emitida el {new Date(b.IssuedAt).toLocaleDateString()}
              </p>
            </div>
            <Link href={`/insignias/${b.VerificationCode}`}>Ver verificación pública</Link>
          </li>
        ))}
      </ul>
    </div>
  );
}
