import { describe, expect, it } from "vitest";
import { blocksToMarkdown, markdownToBlocks, type Block } from "@/components/BlockEditor";

/**
 * El enunciado pide Markdown extendido canónico y recomienda pruebas de ida y
 * vuelta bloqueantes por tipo de nodo. Lo que se fija aquí son dos
 * propiedades, que no son la misma:
 *
 *  1. bloques → Markdown → bloques devuelve los mismos bloques. Si falla, el
 *     profesor pierde estructura al guardar.
 *  2. Markdown → bloques → Markdown es estable a partir de la primera pasada.
 *     Eso es lo que hace que el Markdown guardado sea canónico: dos ediciones
 *     que no cambian nada no producen documentos distintos.
 *
 * Los identificadores de bloque se generan al vuelo y no forman parte del
 * contenido, así que se ignoran al comparar.
 */
function sinId(bloques: Block[]) {
  return bloques.map(({ id, ...resto }) => resto);
}

/** idaYVuelta comprueba la propiedad 1 para un conjunto de bloques. */
function idaYVuelta(bloques: Block[]) {
  return sinId(markdownToBlocks(blocksToMarkdown(bloques)));
}

describe("ida y vuelta por tipo de nodo", () => {
  const casos: Record<string, Block[]> = {
    "encabezado nivel 1": [{ id: "x", type: "heading", level: 1, content: "Introducción" }],
    "encabezado nivel 2": [{ id: "x", type: "heading", level: 2, content: "Objetivos" }],
    "encabezado nivel 3": [{ id: "x", type: "heading", level: 3, content: "Detalle" }],
    párrafo: [{ id: "x", type: "paragraph", content: "Un párrafo normal." }],
    "párrafo con énfasis": [{ id: "x", type: "paragraph", content: "Texto con **negrita** y *cursiva*." }],
    cita: [{ id: "x", type: "callout", content: "Recuerda repasar la unidad anterior." }],
    lista: [{ id: "x", type: "list", content: "Primer punto" }],
    "código sin lenguaje": [{ id: "x", type: "code", content: "hola()", language: "" }],
    "código con lenguaje": [{ id: "x", type: "code", content: "fmt.Println(1)", language: "go" }],
    "código de varias líneas": [{ id: "x", type: "code", content: "linea1\nlinea2\nlinea3", language: "py" }],
  };

  for (const [nombre, bloques] of Object.entries(casos)) {
    it(`conserva un ${nombre}`, () => {
      expect(idaYVuelta(bloques)).toEqual(sinId(bloques));
    });
  }
});

describe("ida y vuelta de documentos mezclados", () => {
  it("conserva un documento con todos los tipos", () => {
    const doc: Block[] = [
      { id: "1", type: "heading", level: 1, content: "Curso" },
      { id: "2", type: "paragraph", content: "Bienvenida al curso." },
      { id: "3", type: "heading", level: 2, content: "Requisitos" },
      { id: "4", type: "list", content: "Saber leer" },
      { id: "5", type: "list", content: "Tener un navegador" },
      { id: "6", type: "callout", content: "El material se actualiza cada semestre." },
      { id: "7", type: "code", content: "go test ./...", language: "sh" },
      { id: "8", type: "paragraph", content: "Cierre." },
    ];
    expect(idaYVuelta(doc)).toEqual(sinId(doc));
  });

  // Varios elementos de lista seguidos son el caso que más fácil se rompe: si
  // la serialización los separa con línea en blanco y el parseo los vuelve a
  // unir, el número de bloques cambia.
  it("conserva una lista de varios elementos", () => {
    const doc: Block[] = [
      { id: "1", type: "list", content: "uno" },
      { id: "2", type: "list", content: "dos" },
      { id: "3", type: "list", content: "tres" },
    ];
    expect(idaYVuelta(doc)).toEqual(sinId(doc));
  });

  it("conserva una cita de varias líneas como bloques separados", () => {
    const doc: Block[] = [
      { id: "1", type: "callout", content: "primera" },
      { id: "2", type: "callout", content: "segunda" },
    ];
    expect(idaYVuelta(doc)).toEqual(sinId(doc));
  });
});

describe("el Markdown guardado es canónico", () => {
  const entradas = [
    "# Título\n\nUn párrafo.",
    "## Sub\n\n- a\n- b\n- c",
    "* asterisco como viñeta",
    "> cita\n\ntexto",
    "```go\nfmt.Println(1)\n```",
    "Párrafo\ncon salto suave",
    "#  espacios  de  sobra  ",
    "",
  ];

  for (const md of entradas) {
    it(`es estable para ${JSON.stringify(md.slice(0, 30))}`, () => {
      const primera = blocksToMarkdown(markdownToBlocks(md));
      const segunda = blocksToMarkdown(markdownToBlocks(primera));
      expect(segunda).toBe(primera);
    });
  }

  it("normaliza la viñeta de asterisco a guion", () => {
    expect(blocksToMarkdown(markdownToBlocks("* uno\n* dos"))).toBe("- uno\n- dos");
  });
});

describe("casos límite", () => {
  it("un documento vacío da un párrafo vacío, no cero bloques", () => {
    expect(markdownToBlocks("")).toHaveLength(1);
    expect(markdownToBlocks("   \n  ")).toHaveLength(1);
  });

  it("un bloque de código sin cerrar no se pierde", () => {
    const bloques = markdownToBlocks("```go\nfmt.Println(1)");
    expect(sinId(bloques)).toEqual([{ type: "code", content: "fmt.Println(1)", language: "go" }]);
  });

  it("el contenido de un bloque de código no se interpreta como Markdown", () => {
    const doc: Block[] = [
      { id: "1", type: "code", content: "# esto no es un encabezado\n- ni una lista\n> ni una cita", language: "" },
    ];
    expect(idaYVuelta(doc)).toEqual(sinId(doc));
  });

  it("conserva las líneas en blanco dentro de un bloque de código", () => {
    const doc: Block[] = [{ id: "1", type: "code", content: "uno\n\ndos", language: "" }];
    expect(idaYVuelta(doc)).toEqual(sinId(doc));
  });

  it("genera identificadores distintos para bloques distintos", () => {
    const bloques = markdownToBlocks("# a\n\n# b\n\n# c");
    const ids = new Set(bloques.map((b) => b.id));
    expect(ids.size).toBe(bloques.length);
  });
});
