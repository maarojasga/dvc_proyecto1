"use client";

import { useCallback, useEffect, useState } from "react";
import { api, ApiError, type AuditEntry, type IframeDestino, type Session, type User } from "@/lib/api";

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
      <header className="page-header">
        <div>
          <h1>Administración</h1>
          <p>Cuentas, roles, sesiones y la bitácora inmutable de la plataforma.</p>
        </div>
      </header>

      {error && (
        <p className="error-banner" role="alert">
          {error}
        </p>
      )}
      {notice && <p className="success-banner">{notice}</p>}

      <InviteTeacherForm onInvited={() => { setNotice("Profesor creado. Se le envió un correo para establecer su contraseña."); load(q); }} />

      <section className="card">
        <header>
          <h2>Usuarios</h2>
          <p>Cambia el rol o el estado de una cuenta, y revisa sus sesiones abiertas.</p>
        </header>
        <form
          role="search"
          className="barra-busqueda"
          onSubmit={(e) => {
            e.preventDefault();
            load(q);
          }}
        >
          <div className="form-field">
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

      <ListaBlancaDeIframes onError={setError} />

      <Auditoria onError={setError} />
    </div>
  );
}

/**
 * Administración de los destinos que se pueden incrustar en un curso.
 *
 * La lista es la única capa que impide que contenido de terceros llegue al
 * navegador del estudiante: el sandbox y la política de permisos lo contienen
 * una vez cargado, pero no deciden si se carga. Por eso quitar una entrada
 * surte efecto de inmediato sobre los cursos ya publicados.
 */
function ListaBlancaDeIframes({ onError }: { onError: (m: string) => void }) {
  const [items, setItems] = useState<IframeDestino[] | null>(null);
  const [sandbox, setSandbox] = useState("");
  const [host, setHost] = useState("");
  const [subdominios, setSubdominios] = useState(false);
  const [permisos, setPermisos] = useState("");
  const [descripcion, setDescripcion] = useState("");
  const [ocupado, setOcupado] = useState(false);

  const cargar = useCallback(async () => {
    try {
      const res = await api.listIframeAllowlist();
      setItems(res.items ?? []);
      setSandbox(res.sandbox);
    } catch (e) {
      onError(e instanceof ApiError ? e.message : "No se pudo cargar la lista de destinos");
    }
  }, [onError]);

  useEffect(() => {
    cargar();
  }, [cargar]);

  async function agregar(e: React.FormEvent) {
    e.preventDefault();
    setOcupado(true);
    try {
      await api.addIframeDestino({
        host,
        include_subdomains: subdominios,
        permissions: permisos,
        description: descripcion,
      });
      setHost("");
      setPermisos("");
      setDescripcion("");
      setSubdominios(false);
      await cargar();
    } catch (err) {
      onError(err instanceof ApiError ? err.message : "No se pudo autorizar el destino");
    } finally {
      setOcupado(false);
    }
  }

  async function quitar(d: IframeDestino) {
    if (!confirm(`¿Quitar ${d.host} de la lista? Los recursos ya publicados que lo usen dejarán de mostrarse.`)) {
      return;
    }
    setOcupado(true);
    try {
      await api.removeIframeDestino(d.id);
      await cargar();
    } catch (err) {
      onError(err instanceof ApiError ? err.message : "No se pudo quitar el destino");
    } finally {
      setOcupado(false);
    }
  }

  return (
    <section className="card">
      <header>
        <h2>Destinos incrustables</h2>
        <p>
          Solo estos dominios pueden aparecer en un recurso de tipo iframe. Todo lo demás se
          rechaza al guardarlo y al servirlo.
        </p>
      </header>

      <form className="barra-busqueda" onSubmit={agregar}>
        <div className="form-field">
          <label htmlFor="iframe-host">Dominio</label>
          <input
            id="iframe-host"
            value={host}
            onChange={(e) => setHost(e.target.value)}
            placeholder="player.vimeo.com"
            required
          />
        </div>
        <div className="form-field">
          <label htmlFor="iframe-permisos">Permisos concedidos</label>
          <input
            id="iframe-permisos"
            value={permisos}
            onChange={(e) => setPermisos(e.target.value)}
            placeholder="fullscreen; picture-in-picture"
          />
        </div>
        <div className="form-field">
          <label htmlFor="iframe-desc">Para qué</label>
          <input
            id="iframe-desc"
            value={descripcion}
            onChange={(e) => setDescripcion(e.target.value)}
            placeholder="Reproductor de vídeo"
          />
        </div>
        <div className="form-field">
          <label htmlFor="iframe-sub">
            <input
              id="iframe-sub"
              type="checkbox"
              checked={subdominios}
              onChange={(e) => setSubdominios(e.target.checked)}
            />{" "}
            Incluir subdominios
          </label>
        </div>
        <button type="submit" disabled={ocupado || !host.trim()}>
          Autorizar
        </button>
      </form>

      {subdominios && (
        <p className="warning-banner" role="status">
          Incluir subdominios autoriza también los que ese tercero cree en el futuro, incluido
          cualquiera que aloje contenido de sus usuarios.
        </p>
      )}

      {items === null && <p role="status">Cargando…</p>}
      {items?.length === 0 && (
        <div className="estado-vacio">
          <p>No hay ningún destino autorizado.</p>
          <p>Mientras la lista esté vacía, no se puede guardar ningún recurso incrustado.</p>
        </div>
      )}

      {items && items.length > 0 && (
        <ul className="lista-filas">
          {items.map((d) => (
            <li key={d.id} className="fila">
              <div className="fila__datos">
                <p>
                  <strong>{d.host}</strong>{" "}
                  {d.include_subdomains && <span className="badge">y subdominios</span>}{" "}
                  {d.permissions ? (
                    <span className="badge">{d.permissions}</span>
                  ) : (
                    <span className="badge">sin permisos extra</span>
                  )}
                </p>
                {d.description && <p className="muted">{d.description}</p>}
              </div>
              <button className="secondary" disabled={ocupado} onClick={() => quitar(d)}>
                Quitar
              </button>
            </li>
          ))}
        </ul>
      )}

      {sandbox && (
        <p className="muted">
          Todo marco se sirve con <code>sandbox=&quot;{sandbox}&quot;</code>, que le niega navegar la
          ventana principal, abrir descargas y mostrar diálogos modales.
        </p>
      )}
    </section>
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
      <header>
        <h2>Auditoría</h2>
        <p>Registro inmutable: la base rechaza modificarlo o borrarlo.</p>
      </header>

      <form
        role="search"
        className="barra-busqueda"
        onSubmit={(e) => {
          e.preventDefault();
          void cargar(accion);
        }}
      >
        <div className="form-field">
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
      {entradas && entradas.length === 0 && (
        <div className="estado-vacio">
          <p>Sin entradas para ese filtro.</p>
        </div>
      )}
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
      <header>
        <h2>Invitar profesor</h2>
        <p>Los profesores no se autorregistran: reciben un enlace para fijar su clave.</p>
      </header>
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
