"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import Link from "next/link";
import { api, type Version, ApiError } from "@/lib/api";
import { useI18n, useTraductorEstable } from "@/lib/i18n";

// Los filtros del catálogo (punto 10 del alcance mínimo) los resuelve la API:
// aquí solo se recogen y se mandan como parámetros de consulta. Filtrar en el
// navegador sobre la página ya cargada daría resultados incompletos en cuanto
// el catálogo pase de una página.

interface Filtros {
  q: string;
  category: string;
  level: string;
}

const SIN_FILTROS: Filtros = { q: "", category: "", level: "" };

export default function HomePage() {
  const { t } = useI18n();
  const traducir = useTraductorEstable();
  const [courses, setCourses] = useState<Version[] | null>(null);
  const [error, setError] = useState("");
  const [filtros, setFiltros] = useState<Filtros>(SIN_FILTROS);
  // Lo aplicado, que puede diferir de lo escrito hasta que se pulsa Buscar.
  const [aplicados, setAplicados] = useState<Filtros>(SIN_FILTROS);
  // Las opciones salen del propio catálogo: no hay taxonomía fija, cada
  // profesor escribe su categoría y su nivel al publicar.
  const [opciones, setOpciones] = useState<{ categorias: string[]; niveles: string[] }>({
    categorias: [],
    niveles: [],
  });

  const cargar = useCallback(async (f: Filtros) => {
    setError("");
    const params: Record<string, string> = {};
    if (f.q) params.q = f.q;
    if (f.category) params.category = f.category;
    if (f.level) params.level = f.level;
    try {
      const res = await api.listCatalog(params);
      setCourses(res.items ?? []);
    } catch (e) {
      setError(e instanceof ApiError ? e.message : traducir("catalogo.error"));
    }
  }, [traducir]);

  useEffect(() => {
    cargar(SIN_FILTROS);
    // Las opciones se toman del catálogo completo una sola vez, para que
    // filtrar por categoría no haga desaparecer las demás del desplegable.
    api
      .listCatalog({})
      .then((res) => {
        const items = res.items ?? [];
        setOpciones({
          categorias: unicos(items.map((v) => v.Category)),
          niveles: unicos(items.map((v) => v.Level)),
        });
      })
      .catch(() => setOpciones({ categorias: [], niveles: [] }));
  }, [cargar]);

  const hayFiltros = useMemo(
    () => Boolean(aplicados.q || aplicados.category || aplicados.level),
    [aplicados],
  );

  function aplicar(f: Filtros) {
    setFiltros(f);
    setAplicados(f);
    setCourses(null);
    cargar(f);
  }

  return (
    <div>
      <header className="page-header">
        <div>
          <h1>{t("catalogo.titulo")}</h1>
          <p>{t("catalogo.subtitulo")}</p>
        </div>
      </header>

      <form
        role="search"
        aria-label="Buscar y filtrar cursos"
        className="barra-busqueda"
        onSubmit={(e) => {
          e.preventDefault();
          aplicar(filtros);
        }}
      >
        <div className="form-field">
          <label htmlFor="q">{t("catalogo.buscar")}</label>
          <input
            id="q"
            value={filtros.q}
            onChange={(e) => setFiltros({ ...filtros, q: e.target.value })}
            placeholder={t("catalogo.buscarPlaceholder")}
          />
        </div>

        <div className="form-field">
          <label htmlFor="categoria">{t("catalogo.categoria")}</label>
          <select
            id="categoria"
            value={filtros.category}
            onChange={(e) => aplicar({ ...filtros, category: e.target.value })}
          >
            <option value="">{t("catalogo.todas")}</option>
            {opciones.categorias.map((c) => (
              <option key={c} value={c}>
                {c}
              </option>
            ))}
          </select>
        </div>

        <div className="form-field">
          <label htmlFor="nivel">{t("catalogo.nivel")}</label>
          <select
            id="nivel"
            value={filtros.level}
            onChange={(e) => aplicar({ ...filtros, level: e.target.value })}
          >
            <option value="">{t("catalogo.todos")}</option>
            {opciones.niveles.map((n) => (
              <option key={n} value={n}>
                {n}
              </option>
            ))}
          </select>
        </div>

        <button type="submit">{t("catalogo.buscarBoton")}</button>
        {hayFiltros && (
          <button type="button" className="secondary" onClick={() => aplicar(SIN_FILTROS)}>
            {t("catalogo.limpiar")}
          </button>
        )}
      </form>

      {error && (
        <p className="error-banner" role="alert">
          {error}
        </p>
      )}

      {courses === null && !error && <p role="status">{t("catalogo.cargando")}</p>}

      {courses && (
        <p role="status" className="muted">
          {courses.length === 1 ? t("catalogo.unCurso") : t("catalogo.nCursos", { n: courses.length })}
          {hayFiltros ? t("catalogo.conFiltros") : t("catalogo.publicados")}
        </p>
      )}

      {courses?.length === 0 && (
        <div className="estado-vacio">
          <p>{t("catalogo.sinResultados")}</p>
          <p>{t("catalogo.pruebaOtro")}</p>
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
    </div>
  );
}

function unicos(valores: (string | undefined | null)[]): string[] {
  return Array.from(new Set(valores.filter((v): v is string => Boolean(v && v.trim())))).sort((a, b) =>
    a.localeCompare(b, "es"),
  );
}
