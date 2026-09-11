"use client";

import { useEffect, useState } from "react";
import Link from "next/link";
import { api, type Version, ApiError } from "@/lib/api";

/** NIVELES y las categorías vienen del propio catálogo; el nivel es cerrado. */
const NIVELES: [string, string][] = [
  ["", "Cualquier nivel"],
  ["basico", "Básico"],
  ["intermedio", "Intermedio"],
  ["avanzado", "Avanzado"],
];

export default function HomePage() {
  const [courses, setCourses] = useState<Version[] | null>(null);
  const [error, setError] = useState("");
  const [q, setQ] = useState("");
  const [categoria, setCategoria] = useState("");
  const [nivel, setNivel] = useState("");
  // Cursor de la página siguiente. Vacío significa que no hay más.
  const [siguiente, setSiguiente] = useState("");
  const [cargandoMas, setCargandoMas] = useState(false);

  useEffect(() => {
    load();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  /**
   * load pide una página. Con cursor añade a lo que ya hay; sin él empieza de
   * nuevo, que es lo que corresponde al cambiar un filtro.
   */
  async function load(cursor = "") {
    setError("");
    if (cursor) setCargandoMas(true);
    const params: Record<string, string> = {};
    if (q) params.q = q;
    if (categoria) params.category = categoria;
    if (nivel) params.level = nivel;
    if (cursor) params.cursor = cursor;
    try {
      const res = await api.listCatalog(params);
      setCourses((previos) => (cursor && previos ? [...previos, ...(res.items ?? [])] : (res.items ?? [])));
      setSiguiente(res.next_cursor ?? "");
    } catch (e) {
      setError(e instanceof ApiError ? e.message : "No se pudo cargar el catálogo");
    } finally {
      setCargandoMas(false);
    }
  }

  /** categoriasVistas sale de lo cargado: el catálogo no expone un listado. */
  const categoriasVistas = Array.from(
    new Set((courses ?? []).map((v) => v.Category).filter((c): c is string => Boolean(c))),
  ).sort();

  return (
    <div>
      <header className="page-header">
        <div>
          <h1>Catálogo de cursos</h1>
          <p>Cursos publicados y abiertos a inscripción.</p>
        </div>
      </header>

      <form
        role="search"
        className="barra-busqueda"
        onSubmit={(e) => {
          e.preventDefault();
          load();
        }}
      >
        <div className="form-field">
          <label htmlFor="q">Buscar cursos</label>
          <input id="q" value={q} onChange={(e) => setQ(e.target.value)} placeholder="Título o resumen" />
        </div>
        <div className="form-field">
          <label htmlFor="categoria">Categoría</label>
          <select id="categoria" value={categoria} onChange={(e) => setCategoria(e.target.value)}>
            <option value="">Cualquier categoría</option>
            {categoriasVistas.map((c) => (
              <option key={c} value={c}>
                {c}
              </option>
            ))}
          </select>
        </div>
        <div className="form-field">
          <label htmlFor="nivel">Nivel</label>
          <select id="nivel" value={nivel} onChange={(e) => setNivel(e.target.value)}>
            {NIVELES.map(([v, t]) => (
              <option key={v} value={v}>
                {t}
              </option>
            ))}
          </select>
        </div>
        <button type="submit">Buscar</button>
      </form>

      {error && (
        <p className="error-banner" role="alert">
          {error}
        </p>
      )}

      {courses === null && !error && <p role="status">Cargando cursos…</p>}

      {courses?.length === 0 && (
        <div className="estado-vacio">
          <p>No hay cursos publicados que coincidan con la búsqueda.</p>
          <p>Prueba con otro término o quita algún filtro.</p>
        </div>
      )}

      <ul className="rejilla">
        {courses?.map((v) => (
          <li key={v.ID}>
            <article className="card">
              <h2>
                <Link href={`/cursos/${v.CourseID}`}>{v.Title}</Link>
              </h2>
              <p style={{ flex: 1 }}>{v.Summary}</p>
              <p className="row">
                {v.Category && <span className="badge">{v.Category}</span>}
                {v.Level && <span className="badge">{v.Level}</span>}
                <span className="badge">{v.Language}</span>
              </p>
            </article>
          </li>
        ))}
      </ul>

      {siguiente && (
        <p>
          <button type="button" onClick={() => load(siguiente)} disabled={cargandoMas}>
            {cargandoMas ? "Cargando…" : "Cargar más cursos"}
          </button>
        </p>
      )}
    </div>
  );
}
