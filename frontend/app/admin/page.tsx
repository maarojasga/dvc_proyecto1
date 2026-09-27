"use client";

import { useCallback, useEffect, useState } from "react";
import {
  api,
  ApiError,
  type AuditEntry,
  type Conteo,
  type IframeDestino,
  type Metricas,
  type Session,
  type User,
} from "@/lib/api";
import { useI18n, type Clave } from "@/lib/i18n";

const ROLES: User["role"][] = ["student", "teacher", "admin"];
const STATUSES: User["status"][] = ["pending_verification", "active", "suspended"];

export default function AdminDashboardPage() {
  const { t, locale } = useI18n();
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
      setError(e instanceof ApiError ? e.message : t("admin.errorUsuarios"));
    }
  }

  async function handleRoleChange(id: string, role: string) {
    setError("");
    try {
      await api.updateUserRole(id, role);
      load(q);
    } catch (e) {
      setError(e instanceof ApiError ? e.message : t("admin.errorRol"));
    }
  }

  async function handleStatusChange(id: string, status: string) {
    setError("");
    try {
      await api.updateUserStatus(id, status);
      load(q);
    } catch (e) {
      setError(e instanceof ApiError ? e.message : t("admin.errorEstado"));
    }
  }

  return (
    <div>
      <header className="page-header">
        <div>
          <h1>{t("nav.administracion")}</h1>
          <p>{t("admin.subtitulo")}</p>
        </div>
      </header>

      {error && (
        <p className="error-banner" role="alert">
          {error}
        </p>
      )}
      {notice && <p className="success-banner">{notice}</p>}

      <InviteTeacherForm
        onInvited={() => {
          setNotice(t("admin.profesorCreado"));
          load(q);
        }}
      />

      <section className="card">
        <header>
          <h2>{t("admin.usuarios")}</h2>
          <p>{t("admin.usuariosAyuda")}</p>
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
            <label htmlFor="q">{t("admin.buscarUsuario")}</label>
            <input id="q" value={q} onChange={(e) => setQ(e.target.value)} />
          </div>
          <button type="submit">{t("comun.buscar")}</button>
        </form>

        {users === null && <p>{t("comun.cargando")}</p>}
        {users && (
          <div style={{ overflowX: "auto" }}>
            <table>
              <thead>
                <tr>
                  <th>{t("admin.colNombre")}</th>
                  <th>{t("admin.colCorreo")}</th>
                  <th>{t("admin.colRol")}</th>
                  <th>{t("admin.colEstado")}</th>
                  <th>{t("admin.colSesiones")}</th>
                </tr>
              </thead>
              <tbody>
                {users.map((u) => (
                  <tr key={u.id}>
                    <td>{u.full_name}</td>
                    <td>{u.email}</td>
                    <td>
                      <label className="visually-hidden" htmlFor={`role-${u.id}`}>
                        {t("admin.rolDe", { nombre: u.full_name })}
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
                        {t("admin.estadoDe", { nombre: u.full_name })}
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

      <PanelDeMetricas onError={setError} />

      <ListaBlancaDeIframes onError={setError} />

      <Auditoria onError={setError} />
    </div>
  );
}

/**
 * Métricas agregadas de la plataforma.
 *
 * El desglose se pinta recorriendo el mapa que devuelve la API en lugar de
 * enumerar estados aquí: así añadir un estado en la base aparece solo, sin
 * tocar el cliente.
 */
function PanelDeMetricas({ onError }: { onError: (m: string) => void }) {
  const { t, locale } = useI18n();
  const [metricas, setMetricas] = useState<Metricas | null>(null);

  useEffect(() => {
    api
      .platformMetrics()
      .then(setMetricas)
      .catch((e) => onError(e instanceof ApiError ? e.message : t("admin.errorMetricas")));
  }, [onError, t]);

  if (!metricas) return null;

  const bloques: [Clave, Conteo][] = [
    ["admin.metricaUsuarios", metricas.usuarios],
    ["admin.metricaCursos", metricas.cursos],
    ["admin.metricaInscripciones", metricas.inscripciones],
    ["admin.metricaInsignias", metricas.insignias],
    ["admin.metricaMultimedia", metricas.multimedia],
    ["admin.metricaEvaluaciones", metricas.evaluaciones],
  ];

  return (
    <section className="card">
      <header>
        <h2>{t("admin.metricas")}</h2>
      </header>
      <ul className="rejilla">
        {bloques.map(([clave, conteo]) => (
          <li key={clave}>
            <article className="card">
              <h3 style={{ margin: 0 }}>{t(clave)}</h3>
              <p style={{ fontSize: "2rem", margin: "0.25rem 0", fontWeight: 600 }}>{conteo.total}</p>
              {Object.keys(conteo.desglose).length === 0 ? (
                <p className="muted">{t("admin.sinRegistros")}</p>
              ) : (
                <ul className="stack" style={{ gap: "0.2rem" }}>
                  {Object.entries(conteo.desglose)
                    .sort(([a], [b]) => a.localeCompare(b, locale))
                    .map(([clave, n]) => (
                      <li key={clave} className="muted">
                        {clave}: <strong>{n}</strong>
                      </li>
                    ))}
                </ul>
              )}
            </article>
          </li>
        ))}
      </ul>
    </section>
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
  const { t } = useI18n();
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
      onError(e instanceof ApiError ? e.message : t("admin.errorDestinos"));
    }
  }, [onError, t]);

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
      onError(err instanceof ApiError ? err.message : t("admin.errorAutorizar"));
    } finally {
      setOcupado(false);
    }
  }

  async function quitar(d: IframeDestino) {
    if (!confirm(t("admin.confirmarQuitarDestino", { host: d.host }))) {
      return;
    }
    setOcupado(true);
    try {
      await api.removeIframeDestino(d.id);
      await cargar();
    } catch (err) {
      onError(err instanceof ApiError ? err.message : t("admin.errorQuitarDestino"));
    } finally {
      setOcupado(false);
    }
  }

  return (
    <section className="card">
      <header>
        <h2>{t("admin.destinos")}</h2>
        <p>{t("admin.destinosAyuda")}</p>
      </header>

      <form className="barra-busqueda" onSubmit={agregar}>
        <div className="form-field">
          <label htmlFor="iframe-host">{t("admin.dominio")}</label>
          <input
            id="iframe-host"
            value={host}
            onChange={(e) => setHost(e.target.value)}
            placeholder="player.vimeo.com"
            required
          />
        </div>
        <div className="form-field">
          <label htmlFor="iframe-permisos">{t("admin.permisos")}</label>
          <input
            id="iframe-permisos"
            value={permisos}
            onChange={(e) => setPermisos(e.target.value)}
            placeholder="fullscreen; picture-in-picture"
          />
        </div>
        <div className="form-field">
          <label htmlFor="iframe-desc">{t("admin.paraQue")}</label>
          <input
            id="iframe-desc"
            value={descripcion}
            onChange={(e) => setDescripcion(e.target.value)}
            placeholder={t("admin.paraQuePlaceholder")}
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
            {t("admin.incluirSubdominios")}
          </label>
        </div>
        <button type="submit" disabled={ocupado || !host.trim()}>
          {t("admin.autorizar")}
        </button>
      </form>

      {subdominios && (
        <p className="warning-banner" role="status">
          {t("admin.avisoSubdominios")}
        </p>
      )}

      {items === null && <p role="status">{t("comun.cargando")}</p>}
      {items?.length === 0 && (
        <div className="estado-vacio">
          <p>{t("admin.sinDestinos")}</p>
          <p>{t("admin.sinDestinosAyuda")}</p>
        </div>
      )}

      {items && items.length > 0 && (
        <ul className="lista-filas">
          {items.map((d) => (
            <li key={d.id} className="fila">
              <div className="fila__datos">
                <p>
                  <strong>{d.host}</strong>{" "}
                  {d.include_subdomains && <span className="badge">{t("admin.ySubdominios")}</span>}{" "}
                  {d.permissions ? (
                    <span className="badge">{d.permissions}</span>
                  ) : (
                    <span className="badge">{t("admin.sinPermisosExtra")}</span>
                  )}
                </p>
                {d.description && <p className="muted">{d.description}</p>}
              </div>
              <button className="secondary" disabled={ocupado} onClick={() => quitar(d)}>
                {t("comun.quitar")}
              </button>
            </li>
          ))}
        </ul>
      )}

      {sandbox && (
        <p className="muted">
          {t("admin.sandboxAntes")} <code>sandbox=&quot;{sandbox}&quot;</code>
          {t("admin.sandboxDespues")}
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
  const { t } = useI18n();
  const [sesiones, setSesiones] = useState<Session[] | null>(null);
  const [ocupado, setOcupado] = useState(false);

  async function cargar() {
    setOcupado(true);
    try {
      const { items } = await api.listUserSessions(usuario.id);
      setSesiones(items ?? []);
    } catch (e) {
      onError(e instanceof ApiError ? e.message : t("sesiones.error"));
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
      onError(e instanceof ApiError ? e.message : t("sesiones.errorRevocarTodas"));
    } finally {
      setOcupado(false);
    }
  }

  if (sesiones === null) {
    return (
      <button className="secondary" disabled={ocupado} onClick={cargar}>
        {t("comun.ver")}
      </button>
    );
  }

  return (
    <span className="row">
      <span className="badge">{t("admin.sesionesActivas", { n: sesiones.length })}</span>
      {sesiones.length > 0 && (
        <button className="secondary" disabled={ocupado} onClick={revocar}>
          {t("admin.cerrarTodas")}
        </button>
      )}
    </span>
  );
}

/** Bitácora inmutable de acciones administrativas y de identidad. */
function Auditoria({ onError }: { onError: (m: string) => void }) {
  const { t, locale } = useI18n();
  const [entradas, setEntradas] = useState<AuditEntry[] | null>(null);
  const [accion, setAccion] = useState("");

  const cargar = useCallback(
    async (filtro: string) => {
      try {
        const { items } = await api.listAudit(filtro ? { action: filtro, limit: 50 } : { limit: 50 });
        setEntradas(items ?? []);
      } catch (e) {
        onError(e instanceof ApiError ? e.message : t("admin.errorAuditoria"));
      }
    },
    [onError, t],
  );

  useEffect(() => {
    void cargar("");
  }, [cargar]);

  return (
    <section className="card">
      <header>
        <h2>{t("admin.auditoria")}</h2>
        <p>{t("admin.auditoriaAyuda")}</p>
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
          <label htmlFor="audit-action">{t("admin.filtrarAccion")}</label>
          <input
            id="audit-action"
            value={accion}
            onChange={(e) => setAccion(e.target.value)}
            placeholder="user.status_updated"
          />
        </div>
        <button type="submit">{t("admin.filtrar")}</button>
      </form>

      {entradas === null && <p>{t("comun.cargando")}</p>}
      {entradas && entradas.length === 0 && (
        <div className="estado-vacio">
          <p>{t("admin.sinEntradas")}</p>
        </div>
      )}
      {entradas && entradas.length > 0 && (
        <div style={{ overflowX: "auto" }}>
          <table>
            <thead>
              <tr>
                <th>{t("admin.colCuando")}</th>
                <th>{t("admin.colAccion")}</th>
                <th>{t("admin.colActor")}</th>
                <th>{t("admin.colEntidad")}</th>
                <th>{t("admin.colIP")}</th>
              </tr>
            </thead>
            <tbody>
              {entradas.map((e) => (
                <tr key={e.id}>
                  <td>{new Date(e.created_at).toLocaleString(locale)}</td>
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
  const { t } = useI18n();
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
      setError(e instanceof ApiError ? e.message : t("admin.errorProfesor"));
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <form onSubmit={handleSubmit} className="card stack">
      <header>
        <h2>{t("admin.invitarProfesor")}</h2>
        <p>{t("admin.invitarAyuda")}</p>
      </header>
      {error && <p className="error-banner">{error}</p>}
      <div className="row">
        <div className="form-field" style={{ flex: 1 }}>
          <label htmlFor="teacher-name">{t("auth.nombreCompleto")}</label>
          <input id="teacher-name" required value={fullName} onChange={(e) => setFullName(e.target.value)} />
        </div>
        <div className="form-field" style={{ flex: 1 }}>
          <label htmlFor="teacher-email">{t("admin.colCorreo")}</label>
          <input id="teacher-email" type="email" required value={email} onChange={(e) => setEmail(e.target.value)} />
        </div>
      </div>
      <button type="submit" disabled={submitting}>
        {submitting ? t("profesor.creando") : t("admin.crearProfesor")}
      </button>
    </form>
  );
}
