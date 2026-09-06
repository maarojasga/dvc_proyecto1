"use client";

import { useEffect, useState } from "react";
import { api, type User, ApiError } from "@/lib/api";

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
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </section>
    </div>
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
