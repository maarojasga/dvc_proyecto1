"use client";

import { createContext, useCallback, useContext, useEffect, useMemo, useState } from "react";

// Internacionalización de la interfaz (alcance opcional 5.2).
//
// El catálogo va tipado contra las claves del español, que es el idioma de
// origen: así añadir un texto sin traducirlo es un error de compilación y no
// una cadena que aparece en español dentro de una pantalla en inglés.
//
// Lo que no se traduce aquí es el contenido de los cursos. Ese lo escribe cada
// profesor y ya tiene su propio idioma declarado en la versión
// (`course_versions.language`): traducirlo automáticamente sería inventar
// material didáctico.

export const IDIOMAS = ["es", "en"] as const;
export type Idioma = (typeof IDIOMAS)[number];

export const NOMBRES_DE_IDIOMA: Record<Idioma, string> = {
  es: "Español",
  en: "English",
};

const es = {
  "nav.catalogo": "Catálogo",
  "nav.misCursos": "Mis cursos",
  "nav.misInsignias": "Mis insignias",
  "nav.autoria": "Autoría",
  "nav.administracion": "Administración",
  "nav.sesiones": "Mis sesiones",
  "nav.entrar": "Entrar",
  "nav.registrarse": "Crear cuenta",
  "nav.salir": "Salir",
  "nav.idioma": "Idioma",

  "catalogo.titulo": "Catálogo de cursos",
  "catalogo.subtitulo": "Cursos publicados y abiertos a inscripción.",
  "catalogo.buscar": "Buscar cursos",
  "catalogo.buscarPlaceholder": "Título o resumen",
  "catalogo.categoria": "Categoría",
  "catalogo.todas": "Todas",
  "catalogo.nivel": "Nivel",
  "catalogo.todos": "Todos",
  "catalogo.buscarBoton": "Buscar",
  "catalogo.limpiar": "Limpiar filtros",
  "catalogo.cargando": "Cargando cursos…",
  "catalogo.sinResultados": "No hay cursos publicados que coincidan con la búsqueda.",
  "catalogo.pruebaOtro": "Prueba con otro término o quita algún filtro.",
  "catalogo.unCurso": "1 curso",
  "catalogo.nCursos": "{n} cursos",
  "catalogo.conFiltros": " con los filtros aplicados",
  "catalogo.publicados": " publicados",

  "cuenta.exportar": "Descargar mis datos",
  "cuenta.exportarDescripcion":
    "Un archivo con todo lo que la plataforma guarda sobre ti: cuenta, sesiones, inscripciones, progreso, evaluaciones e insignias.",
  "cuenta.exportando": "Preparando la descarga…",
  "cuenta.exportarError": "No se pudieron descargar tus datos.",

  "coautoria.titulo": "Coautoría",
  "coautoria.descripcion":
    "Otros profesores que pueden editar este curso. Editan el contenido, pero no pueden invitar ni quitar a nadie.",
  "coautoria.correo": "Correo del profesor",
  "coautoria.invitar": "Dar acceso",
  "coautoria.quitar": "Quitar",
  "coautoria.ninguno": "Nadie más tiene acceso a este curso.",
  "coautoria.noEsProfesor":
    "No se pudo dar acceso a ese correo. Solo las cuentas de profesor activas pueden coeditar.",

  "comun.cargando": "Cargando…",
  "comun.cerrar": "Cerrar",
  "comun.guardar": "Guardar",
  "comun.cancelar": "Cancelar",
} as const;

/** Clave es cualquiera de los textos del catálogo de origen. */
export type Clave = keyof typeof es;

// El catálogo en inglés se declara con el mismo tipo, así que falta una clave
// o sobra una inventada, TypeScript lo dice.
const en: Record<Clave, string> = {
  "nav.catalogo": "Catalog",
  "nav.misCursos": "My courses",
  "nav.misInsignias": "My badges",
  "nav.autoria": "Authoring",
  "nav.administracion": "Administration",
  "nav.sesiones": "My sessions",
  "nav.entrar": "Sign in",
  "nav.registrarse": "Create account",
  "nav.salir": "Sign out",
  "nav.idioma": "Language",

  "catalogo.titulo": "Course catalog",
  "catalogo.subtitulo": "Published courses open for enrollment.",
  "catalogo.buscar": "Search courses",
  "catalogo.buscarPlaceholder": "Title or summary",
  "catalogo.categoria": "Category",
  "catalogo.todas": "All",
  "catalogo.nivel": "Level",
  "catalogo.todos": "All",
  "catalogo.buscarBoton": "Search",
  "catalogo.limpiar": "Clear filters",
  "catalogo.cargando": "Loading courses…",
  "catalogo.sinResultados": "No published courses match your search.",
  "catalogo.pruebaOtro": "Try another term or remove a filter.",
  "catalogo.unCurso": "1 course",
  "catalogo.nCursos": "{n} courses",
  "catalogo.conFiltros": " matching the filters",
  "catalogo.publicados": " published",

  "cuenta.exportar": "Download my data",
  "cuenta.exportarDescripcion":
    "A file with everything the platform stores about you: account, sessions, enrollments, progress, assessments and badges.",
  "cuenta.exportando": "Preparing the download…",
  "cuenta.exportarError": "Your data could not be downloaded.",

  "coautoria.titulo": "Co-authoring",
  "coautoria.descripcion":
    "Other teachers who can edit this course. They edit the content, but cannot invite or remove anyone.",
  "coautoria.correo": "Teacher's email",
  "coautoria.invitar": "Grant access",
  "coautoria.quitar": "Remove",
  "coautoria.ninguno": "Nobody else has access to this course.",
  "coautoria.noEsProfesor":
    "Access could not be granted to that address. Only active teacher accounts can co-edit.",

  "comun.cargando": "Loading…",
  "comun.cerrar": "Close",
  "comun.guardar": "Save",
  "comun.cancelar": "Cancel",
};

const CATALOGOS: Record<Idioma, Record<Clave, string>> = { es, en };

const CLAVE_ALMACEN = "mooc.idioma";

interface Contexto {
  idioma: Idioma;
  cambiar: (idioma: Idioma) => void;
  /** t traduce una clave, sustituyendo los marcadores {nombre}. */
  t: (clave: Clave, valores?: Record<string, string | number>) => string;
}

const I18nContext = createContext<Contexto | null>(null);

/**
 * idiomaInicial elige el idioma de arranque.
 *
 * Manda lo que la persona eligió; si no eligió nada, el del navegador; y si
 * tampoco, español, que es el idioma de origen del contenido.
 */
function idiomaInicial(): Idioma {
  if (typeof window === "undefined") return "es";
  try {
    const guardado = localStorage.getItem(CLAVE_ALMACEN);
    if (guardado && (IDIOMAS as readonly string[]).includes(guardado)) {
      return guardado as Idioma;
    }
  } catch {
    /* sin localStorage se usa el del navegador */
  }
  const navegador = navigator.language?.slice(0, 2);
  return (IDIOMAS as readonly string[]).includes(navegador) ? (navegador as Idioma) : "es";
}

export function I18nProvider({ children }: { children: React.ReactNode }) {
  // Se arranca siempre en español y se ajusta tras montar: leer localStorage o
  // navigator durante el render del servidor produce una hidratación distinta
  // de la que sirvió el servidor, y React lo descarta con un aviso.
  const [idioma, setIdioma] = useState<Idioma>("es");

  useEffect(() => {
    setIdioma(idiomaInicial());
  }, []);

  useEffect(() => {
    document.documentElement.lang = idioma;
  }, [idioma]);

  const cambiar = useCallback((nuevo: Idioma) => {
    setIdioma(nuevo);
    try {
      localStorage.setItem(CLAVE_ALMACEN, nuevo);
    } catch {
      /* la elección no sobrevivirá a la recarga, pero la sesión sí */
    }
  }, []);

  const t = useCallback(
    (clave: Clave, valores?: Record<string, string | number>) => {
      const texto = CATALOGOS[idioma][clave] ?? CATALOGOS.es[clave] ?? clave;
      if (!valores) return texto;
      return Object.entries(valores).reduce(
        (acc, [nombre, valor]) => acc.replaceAll(`{${nombre}}`, String(valor)),
        texto,
      );
    },
    [idioma],
  );

  const valor = useMemo(() => ({ idioma, cambiar, t }), [idioma, cambiar, t]);
  return <I18nContext.Provider value={valor}>{children}</I18nContext.Provider>;
}

export function useI18n(): Contexto {
  const ctx = useContext(I18nContext);
  if (!ctx) {
    throw new Error("useI18n debe usarse dentro de I18nProvider");
  }
  return ctx;
}
