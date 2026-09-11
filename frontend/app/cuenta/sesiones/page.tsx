"use client";

import { useCallback, useEffect, useState } from "react";
import { useRouter } from "next/navigation";
import { api, ApiError, type Session } from "@/lib/api";
import { useAuth } from "@/lib/auth-context";

/**
 * Sesiones activas del usuario, con revocación.
 *
 * Es la evidencia de que las sesiones son revocables: la lista sale del
 * servidor y revocar invalida el otro dispositivo en la siguiente petición,
 * sin esperar a que expire nada.
 */
export default function SesionesPage() {
  const router = useRouter();
  const { user, loading: cargandoSesion, refresh } = useAuth();
  const [sesiones, setSesiones] = useState<Session[] | null>(null);
  const [error, setError] = useState("");
  const [ocupado, setOcupado] = useState(false);

  const cargar = useCallback(async () => {
    try {
      const { items } = await api.listSessions();
      setSesiones(items);
    } catch (e) {
      setError(e instanceof ApiError ? e.message : "No se pudieron cargar las sesiones");
    }
  }, []);

  useEffect(() => {
    if (cargandoSesion) return;
    if (!user) {
      router.replace("/login");
      return;
    }
    void cargar();
  }, [cargandoSesion, user, router, cargar]);

  async function revocar(sesion: Session) {
    setError("");
    setOcupado(true);
    try {
      await api.revokeSession(sesion.id);
      if (sesion.current) {
        // Revocar la propia sesión equivale a cerrarla: ya no hay lista que ver.
        await refresh();
        router.replace("/");
        return;
      }
      await cargar();
    } catch (e) {
      setError(e instanceof ApiError ? e.message : "No se pudo revocar la sesión");
    } finally {
      setOcupado(false);
    }
  }

  async function revocarLasDemas() {
    setError("");
    setOcupado(true);
    try {
      await api.revokeOtherSessions();
      await cargar();
    } catch (e) {
      setError(e instanceof ApiError ? e.message : "No se pudieron revocar las sesiones");
    } finally {
      setOcupado(false);
    }
  }

  if (cargandoSesion || sesiones === null) {
    return <p role="status">Cargando sesiones…</p>;
  }

  const otras = sesiones.filter((s) => !s.current).length;

  return (
    <div>
      <header className="page-header">
        <div>
          <h1>Sesiones activas</h1>
          <p>Cada dispositivo con la sesión abierta. Revocar una la invalida de inmediato.</p>
        </div>
        {otras > 0 && (
          <button className="secondary" disabled={ocupado} onClick={revocarLasDemas}>
            Cerrar las demás ({otras})
          </button>
        )}
      </header>

      {error && <p className="error-banner" role="alert">{error}</p>}

      <ul className="lista-filas">
        {sesiones.map((s) => (
          <li key={s.id} className="card fila">
            <div className="fila__datos">
              <strong>{s.current ? "Este dispositivo" : "Otro dispositivo"}</strong>
              <p className="muted">Iniciada: {new Date(s.created_at).toLocaleString("es-CO")}</p>
              <p className="muted">Vence: {new Date(s.expires_at).toLocaleString("es-CO")}</p>
              {s.ip_address && <p className="muted">IP: {s.ip_address}</p>}
              {s.user_agent && (
                <p className="muted" style={{ wordBreak: "break-all" }}>
                  {s.user_agent}
                </p>
              )}
            </div>
            <button className="secondary" disabled={ocupado} onClick={() => revocar(s)}>
              {s.current ? "Cerrar esta sesión" : "Revocar"}
            </button>
          </li>
        ))}
      </ul>
    </div>
  );
}
