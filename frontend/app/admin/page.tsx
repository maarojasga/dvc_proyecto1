"use client";

import { useCallback, useEffect, useState } from "react";
import { api, ApiError, type AuditEntry, type Session, type User } from "@/lib/api";

const ROLES: User["role"][] = ["student", "teacher", "admin"];
const STATUSES: User["status"][] = ["pending_verification", "active", "suspended"];

export default function AdminDashboardPage() {
  const [users, setUsers] = useState<User[] | null>(null);
  const [q, setQ] = useState("");
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");

  useEffect(() => {
    load();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  async function load(search = "") {
    setError("");
    try {
      const res = await api.listUsers(search ? { q: search } : {});
      setUsers(res.items ?? []);
    } catch (e) {
      setError(e instanceof ApiError ? e.message : "No se pudo cargar la lista de usuarios");
    }
  }

  async function handleRoleChange(id: string, role: string) {
    setError("");
    try {
      await api.updateUserRole(id, role);
      load(q);
    } catch (e) {
      setError(e instanceof ApiError ? e.message : "No se pudo actualizar el rol");
    }
  }

  async function handleStatusChange(id: string, status: string) {
    setError("");
    try {
      await api.updateUserStatus(id, status);
      load(q);
    } catch (e) {
      setError(e instanceof ApiError ? e.message : "No se pudo actualizar el estado");
    }
  }

  return (
    <div>
      <h1>Administración</h1>
      {error && (
        <p className="error-banner" role="alert">
          {error}
        </p>
      )}
      {notice && <p className="success-banner">{notice}</p>}

      <InviteTeacherForm onInvited={() => { setNotice("Profesor creado. Se le envió un correo para establecer su contraseña."); load(q); }} />

      <section className="card">
        <h2 style={{ marginTop: 0 }}>Usuarios</h2>
        <form
          role="search"
          className="row"
          onSubmit={(e) => {
            e.preventDefault();
            load(q);
          }}
        >
          <div className="form-field" style={{ marginBottom: 0, flex: 1 }}>
            <label htmlFor="q">Buscar por nombre o correo</label>
            <input id="q" value={q} onChange={(e) => setQ(e.target.value)} />
          </div>
          <button type="submit">Buscar</button>
        </form>

        {users === null && <p>Cargando…</p>}
        {users && (
          <div style={{ overflowX: "auto" }}>
            <table>
              <thead>
                <tr>
                  <th>Nombre</th>
                  <th>Correo</th>
                  <th>Rol</th>
                  <th>Estado</th>
                  <th>Sesiones</th>
                </tr>
              </thead>
              <tbody>
                {users.map((u) => (
                  <tr key={u.id}>
                    <td>{u.full_name}</td>
                    <td>{u.email}</td>
                    <td>
                      <label className="visually-hidden" htmlFor={`role-${u.id}`}>
                        Rol de {u.full_name}
                      </label>
                      <select id={`role-${u.id}`} value={u.role} onChange={(e) => handleRoleChange(u.id, e.target.value)}>
                        {ROLES.map((r) => (
                          <option key={r} value={r}>
                            {r}
                          </option>
                        ))}
                      </select>
                    </td>
                    <td>
                      <label className="visually-hidden" htmlFor={`status-${u.id}`}>
                        Estado de {u.full_name}
                      </label>
                      <select id={`status-${u.id}`} value={u.status} onChange={(e) => handleStatusChange(u.id, e.target.value)}>
                        {STATUSES.map((s) => (
                          <option key={s} value={s}>
                            {s}
                          </option>
                        ))}
                      </select>
                    </td>
                    <td>
                      <SesionesDeUsuario usuario={u} onError={setError} />
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </section>

      <Auditoria onError={setError} />
    </div>
  );
}

/**
 * Sesiones abiertas de una cuenta, con revocación.
 *
 * Cerrarlas expulsa a la persona de todos sus dispositivos sin suspender la
 * cuenta: es la respuesta a un acceso comprometido cuando la cuenta en sí no
 * está en falta.
 */
function SesionesDeUsuario({ usuario, onError }: { usuario: User; onError: (m: string) => void }) {
  const [sesiones, setSesiones] = useState<Session[] | null>(null);
  const [ocupado, setOcupado] = useState(false);

  async function cargar() {
    setOcupado(true);
    try {
      const { items } = await api.listUserSessions(usuario.id);
      setSesiones(items ?? []);
    } catch (e) {
      onError(e instanceof ApiError ? e.message : "No se pudieron cargar las sesiones");
    } finally {
      setOcupado(false);
    }
  }

  async function revocar() {
    setOcupado(true);
    try {
      await api.revokeUserSessions(usuario.id);
      await cargar();
    } catch (e) {
      onError(e instanceof ApiError ? e.message : "No se pudieron revocar las sesiones");
    } finally {
      setOcupado(false);
    }
  }

  if (sesiones === null) {
    return (
      <button className="secondary" disabled={ocupado} onClick={cargar}>
        Ver
      </button>
    );
  }

  return (
    <span className="row">
      <span className="badge">{sesiones.length} activas</span>
      {sesiones.length > 0 && (
        <button className="secondary" disabled={ocupado} onClick={revocar}>
          Cerrar todas
        </button>
      )}
    </span>
  );
}

/** Bitácora inmutable de acciones administrativas y de identidad. */
function Auditoria({ onError }: { onError: (m: string) => void }) {
  const [entradas, setEntradas] = useState<AuditEntry[] | null>(null);
  const [accion, setAccion] = useState("");

  const cargar = useCallback(
    async (filtro: string) => {
      try {
        const { items } = await api.listAudit(filtro ? { action: filtro, limit: 50 } : { limit: 50 });
        setEntradas(items ?? []);
      } catch (e) {
        onError(e instanceof ApiError ? e.message : "No se pudo cargar la auditoría");
      }
    },
    [onError],
  );

  useEffect(() => {
    void cargar("");
  }, [cargar]);

  return (
    <section className="card">
      <h2 style={{ marginTop: 0 }}>Auditoría</h2>
      <p className="muted">
        Registro inmutable: la base rechaza modificarlo o borrarlo.
      </p>

      <form
        role="search"
        className="row"
        onSubmit={(e) => {
          e.preventDefault();
          void cargar(accion);
        }}
      >
        <div className="form-field" style={{ marginBottom: 0, flex: 1 }}>
          <label htmlFor="audit-action">Filtrar por acción</label>
          <input
            id="audit-action"
            value={accion}
            onChange={(e) => setAccion(e.target.value)}
            placeholder="user.status_updated"
          />
        </div>
        <button type="submit">Filtrar</button>
      </form>

      {entradas === null && <p>Cargando…</p>}
      {entradas && entradas.length === 0 && <p className="muted">Sin entradas.</p>}
      {entradas && entradas.length > 0 && (
        <div style={{ overflowX: "auto" }}>
          <table>
            <thead>
              <tr>
                <th>Cuándo</th>
                <th>Acción</th>
                <th>Actor</th>
                <th>Entidad</th>
                <th>IP</th>
              </tr>
            </thead>
            <tbody>
              {entradas.map((e) => (
                <tr key={e.id}>
                  <td>{new Date(e.created_at).toLocaleString("es-CO")}</td>
                  <td><code>{e.action}</code></td>
                  <td>{e.actor_email || "—"}</td>
                  <td>{e.entity_type}</td>
                  <td>{e.ip_address || "—"}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </section>
  );
}

function InviteTeacherForm({ onInvited }: { onInvited: () => void }) {
  const [email, setEmail] = useState("");
  const [fullName, setFullName] = useState("");
  const [error, setError] = useState("");
  const [submitting, setSubmitting] = useState(false);

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    setError("");
    setSubmitting(true);
    try {
      await api.createTeacher({ email, full_name: fullName });
      setEmail("");
      setFullName("");
      onInvited();
    } catch (e) {
      setError(e instanceof ApiError ? e.message : "No se pudo crear el profesor");
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <form onSubmit={handleSubmit} className="card stack">
      <h2 style={{ marginTop: 0 }}>Invitar profesor</h2>
      {error && <p className="error-banner">{error}</p>}
      <div className="row">
        <div className="form-field" style={{ flex: 1 }}>
          <label htmlFor="teacher-name">Nombre completo</label>
          <input id="teacher-name" required value={fullName} onChange={(e) => setFullName(e.target.value)} />
        </div>
        <div className="form-field" style={{ flex: 1 }}>
          <label htmlFor="teacher-email">Correo</label>
          <input id="teacher-email" type="email" required value={email} onChange={(e) => setEmail(e.target.value)} />
        </div>
      </div>
      <button type="submit" disabled={submitting}>
        {submitting ? "Creando…" : "Crear profesor"}
      </button>
    </form>
  );
}
