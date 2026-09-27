// Conversión entre el AST del editor de bloques y el Markdown extendido
// canónico que se persiste.
//
// El enunciado recomienda prototipar esta biyección con pruebas de ida y
// vuelta bloqueantes por tipo de nodo, y es un buen consejo: mientras el
// editor y el almacenamiento hablen idiomas distintos, cada tipo de bloque
// nuevo es una ocasión de perder contenido en silencio. Aquí "canónico"
// significa que `aMarkdown(aBloques(md))` es estable —aplicarlo dos veces da
// lo mismo que una— aunque el Markdown de entrada admitiera otras grafías.
//
// Vive fuera del componente a propósito: es lógica pura, se prueba sin montar
// React y se puede reutilizar desde cualquier pantalla que edite contenido.

export type TipoDeBloque =
  | "heading"
  | "paragraph"
  | "code"
  | "callout"
  | "list"
  | "table"
  | "formula"
  | "task";

export interface Block {
  id: string;
  type: TipoDeBloque;
  /** Nivel del encabezado (1-3). Solo en "heading". */
  level?: 1 | 2 | 3;
  /**
   * Texto del bloque. En "table" son las filas separadas por saltos de línea
   * y las celdas por "|"; en "list" y "task", un elemento por línea.
   */
  content: string;
  /** Lenguaje del bloque de código. */
  language?: string;
  /** Marcado de cada tarea, en el mismo orden que las líneas de content. */
  done?: boolean[];
}

let contador = 0;

/** nuevoID genera identificadores estables dentro de una sesión de edición. */
export function nuevoID(): string {
  contador += 1;
  return `b-${contador}-${Math.random().toString(36).slice(2, 6)}`;
}

const VACIO: Block[] = [];

/**
 * aBloques interpreta Markdown extendido y devuelve el AST del editor.
 *
 * Lo que no reconoce no se pierde: cae en un párrafo, que al volver a
 * serializarse sale tal cual. Es la única forma de que abrir un contenido
 * escrito a mano en el editor no lo mutile.
 */
export function aBloques(md: string): Block[] {
  if (!md || !md.trim()) return [...VACIO];

  const lineas = md.replace(/\r\n/g, "\n").split("\n");
  const bloques: Block[] = [];

  let parrafo: string[] = [];
  const volcarParrafo = () => {
    const texto = parrafo.join("\n").trim();
    if (texto) bloques.push({ id: nuevoID(), type: "paragraph", content: texto });
    parrafo = [];
  };

  for (let i = 0; i < lineas.length; i++) {
    const linea = lineas[i];

    // Código: se consume hasta el cierre para no interpretar su interior.
    if (linea.startsWith("```")) {
      volcarParrafo();
      const lenguaje = linea.slice(3).trim();
      const cuerpo: string[] = [];
      i++;
      while (i < lineas.length && !lineas[i].startsWith("```")) {
        cuerpo.push(lineas[i]);
        i++;
      }
      bloques.push({ id: nuevoID(), type: "code", content: cuerpo.join("\n"), language: lenguaje });
      continue;
    }

    // Fórmula en bloque ($$ … $$), la notación de LaTeX que entienden KaTeX
    // y MathJax.
    if (linea.trim() === "$$") {
      volcarParrafo();
      const cuerpo: string[] = [];
      i++;
      while (i < lineas.length && lineas[i].trim() !== "$$") {
        cuerpo.push(lineas[i]);
        i++;
      }
      bloques.push({ id: nuevoID(), type: "formula", content: cuerpo.join("\n").trim() });
      continue;
    }

    // Tabla: una cabecera seguida de la fila de separación con guiones.
    if (esFilaDeTabla(linea) && i + 1 < lineas.length && esSeparadorDeTabla(lineas[i + 1])) {
      volcarParrafo();
      const filas: string[] = [celdasDe(linea).join("|")];
      i += 2; // se salta el separador, que se regenera al serializar
      while (i < lineas.length && esFilaDeTabla(lineas[i])) {
        filas.push(celdasDe(lineas[i]).join("|"));
        i++;
      }
      i--;
      bloques.push({ id: nuevoID(), type: "table", content: filas.join("\n") });
      continue;
    }

    // Lista de tareas: se comprueba antes que la lista normal, porque
    // "- [ ] x" también empieza por "- ".
    if (esTarea(linea)) {
      volcarParrafo();
      const textos: string[] = [];
      const marcas: boolean[] = [];
      while (i < lineas.length && esTarea(lineas[i])) {
        marcas.push(/^[-*] \[[xX]\] /.test(lineas[i].trim()));
        textos.push(lineas[i].trim().replace(/^[-*] \[[ xX]\] /, ""));
        i++;
      }
      i--;
      bloques.push({ id: nuevoID(), type: "task", content: textos.join("\n"), done: marcas });
      continue;
    }

    // Lista: las líneas contiguas forman un solo bloque, que es como se
    // editan y como se leen.
    if (/^[-*] /.test(linea)) {
      volcarParrafo();
      const elementos: string[] = [];
      while (i < lineas.length && /^[-*] /.test(lineas[i]) && !esTarea(lineas[i])) {
        elementos.push(lineas[i].slice(2).trim());
        i++;
      }
      i--;
      bloques.push({ id: nuevoID(), type: "list", content: elementos.join("\n") });
      continue;
    }

    if (linea.startsWith("> ")) {
      volcarParrafo();
      const partes: string[] = [];
      while (i < lineas.length && lineas[i].startsWith("> ")) {
        partes.push(lineas[i].slice(2).trim());
        i++;
      }
      i--;
      bloques.push({ id: nuevoID(), type: "callout", content: partes.join("\n") });
      continue;
    }

    const encabezado = /^(#{1,3}) (.*)$/.exec(linea);
    if (encabezado) {
      volcarParrafo();
      bloques.push({
        id: nuevoID(),
        type: "heading",
        level: encabezado[1].length as 1 | 2 | 3,
        content: encabezado[2].trim(),
      });
      continue;
    }

    if (linea.trim() === "") {
      volcarParrafo();
      continue;
    }
    parrafo.push(linea);
  }

  volcarParrafo();
  return bloques;
}

/**
 * aMarkdown serializa el AST a la forma canónica.
 *
 * Un bloque vacío no produce nada: un editor acumula bloques en blanco
 * mientras se escribe, y persistirlos llenaría el documento de líneas
 * huérfanas que reaparecen en cada ida y vuelta.
 */
export function aMarkdown(bloques: Block[]): string {
  return bloques
    .filter((b) => b.content.trim().length > 0)
    .map(serializar)
    .filter((s) => s.trim().length > 0)
    .join("\n\n");
}

function serializar(b: Block): string {
  switch (b.type) {
    case "heading":
      return `${"#".repeat(b.level ?? 2)} ${b.content.trim()}`;
    case "code":
      return `\`\`\`${b.language ?? ""}\n${b.content}\n\`\`\``;
    case "formula":
      return `$$\n${b.content.trim()}\n$$`;
    case "callout":
      return lineasDe(b.content)
        .map((l) => `> ${l}`)
        .join("\n");
    case "list":
      return lineasDe(b.content)
        .map((l) => `- ${l}`)
        .join("\n");
    case "task":
      return lineasDe(b.content)
        .map((l, i) => `- [${b.done?.[i] ? "x" : " "}] ${l}`)
        .join("\n");
    case "table":
      return serializarTabla(b.content);
    default:
      return b.content.trim();
  }
}

// serializarTabla regenera la fila de separación, que es lo que hace canónica
// la salida: da igual con cuántos guiones se escribiera en el original.
function serializarTabla(contenido: string): string {
  const filas = lineasDe(contenido).map((f) => f.split("|").map((c) => c.trim()));
  if (filas.length === 0) return "";
  const columnas = Math.max(...filas.map((f) => f.length));

  const pintar = (celdas: string[]) => {
    const completas = [...celdas];
    while (completas.length < columnas) completas.push("");
    return `| ${completas.join(" | ")} |`;
  };

  const separador = `| ${Array(columnas).fill("---").join(" | ")} |`;
  return [pintar(filas[0]), separador, ...filas.slice(1).map(pintar)].join("\n");
}

function lineasDe(contenido: string): string[] {
  return contenido
    .split("\n")
    .map((l) => l.trim())
    .filter((l) => l.length > 0);
}

// esTarea reconoce "- [ ] texto" y "- [x] texto". Se comprueba antes que la
// lista normal porque toda tarea es también, sintácticamente, un elemento de
// lista.
function esTarea(linea: string): boolean {
  return /^[-*] \[[ xX]\] /.test(linea.trim());
}

function esFilaDeTabla(linea: string): boolean {
  const t = linea.trim();
  return t.startsWith("|") && t.endsWith("|") && t.length > 2;
}

function esSeparadorDeTabla(linea: string): boolean {
  const t = linea.trim();
  return esFilaDeTabla(t) && /^\|[\s:|-]+\|$/.test(t) && t.includes("-");
}

function celdasDe(linea: string): string[] {
  return linea
    .trim()
    .slice(1, -1)
    .split("|")
    .map((c) => c.trim());
}

/** bloqueVacio crea el bloque con que arranca un editor sin contenido. */
export function bloqueVacio(): Block {
  return { id: nuevoID(), type: "paragraph", content: "" };
}
