"use client";

import { useCallback, useEffect, useState } from "react";

import { api, ApiError, type Colaborador } from "@/lib/api";
import { useI18n } from "@/lib/i18n";

/**
 * Gestión de los coautores de un curso.
 *
 * Solo la ve el dueño: un colaborador edita el contenido, pero repartir el
 * acceso no se delega. El servidor lo impone igual; esto es no enseñar lo que
 * de todas formas no se puede hacer.
 */
export function Coautoria({ courseId }: { courseId: string }) {
  const { t } = useI18n();
  const [items, setItems] = useState<Colaborador[] | null>(null);
  const [correo, setCorreo] = useState("");
  const [error, setError] = useState("");
  const [ocupado, setOcupado] = useState(false);

  const cargar = useCallback(async () => {
    try {
      const res = await api.listCollaborators(courseId);
      setItems(res.items ?? []);
    } catch {
      // Un colaborador no puede leer la lista: entonces esta sección no le
      // corresponde y simplemente no se pinta.
      setItems(null);
    }
  }, [courseId]);

  useEffect(() => {
    cargar();
  }, [cargar]);

  if (items === null) return null;

  async function invitar(e: React.FormEvent) {
    e.preventDefault();
    setOcupado(true);
    setError("");
    try {
      await api.addCollaborator(courseId, correo);
      setCorreo("");
      await cargar();
    } catch (err) {
      setError(
        err instanceof ApiError && err.code === "teacher_not_found"
          ? t("coautoria.noEsProfesor")
          : err instanceof ApiError
            ? err.message
            : t("coautoria.noEsProfesor"),
      );
    } finally {
      setOcupado(false);
    }
  }

  async function quitar(c: Colaborador) {
    setOcupado(true);
    try {
      await api.removeCollaborator(courseId, c.user_id);
      await cargar();
    } catch (err) {
      setError(err instanceof ApiError ? err.message : t("coautoria.errorQuitar"));
    } finally {
      setOcupado(false);
    }
  }

  return (
    <section className="card stack">
      <header>
        <h2>{t("coautoria.titulo")}</h2>
        <p>{t("coautoria.descripcion")}</p>
      </header>

      {error && (
        <p className="error-banner" role="alert">
          {error}
        </p>
      )}

      <form className="barra-busqueda" onSubmit={invitar}>
        <div className="form-field">
          <label htmlFor="colaborador-correo">{t("coautoria.correo")}</label>
          <input
            id="colaborador-correo"
            type="email"
            value={correo}
            onChange={(e) => setCorreo(e.target.value)}
            required
          />
        </div>
        <button type="submit" disabled={ocupado || !correo.trim()}>
          {t("coautoria.invitar")}
        </button>
      </form>

      {items.length === 0 ? (
        <p className="muted">{t("coautoria.ninguno")}</p>
      ) : (
        <ul className="lista-filas">
          {items.map((c) => (
            <li key={c.user_id} className="fila">
              <div className="fila__datos">
                <p>
                  <strong>{c.full_name}</strong> <span className="muted">{c.email}</span>
                </p>
                <p className="muted">
                  {c.role} · {new Date(c.added_at).toLocaleDateString()}
                </p>
              </div>
              <button className="secondary" disabled={ocupado} onClick={() => quitar(c)}>
                {t("coautoria.quitar")}
              </button>
            </li>
          ))}
        </ul>
      )}
    </section>
  );
}
