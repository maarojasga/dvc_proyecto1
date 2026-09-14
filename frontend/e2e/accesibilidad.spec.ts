import { test, expect } from "@playwright/test";
import AxeBuilder from "@axe-core/playwright";
import {
  API,
  conToken,
  credencialesDeAdmin,
  entrarPorLaInterfaz,
  registrarEstudiante,
  sufijo,
  tokenDeSesion,
} from "./ayudas";
import { publicarCursoDePrueba } from "./preparar-curso";

/**
 * Auditoría automática de accesibilidad (condición de aceptación, sección 10).
 *
 * "No puede presentar incumplimientos críticos". Aquí eso se concreta en: cero
 * violaciones de las reglas WCAG 2.2 A y AA que axe sabe comprobar, en las
 * pantallas por las que pasa un usuario real.
 *
 * Lo que esta auditoría **no** demuestra: axe detecta del orden de la mitad de
 * los problemas de accesibilidad reales. Que no encuentre nada no significa
 * que la plataforma sea accesible, significa que no tiene los defectos que una
 * máquina puede ver. El orden de foco, el texto alternativo que es correcto
 * pero inútil y la navegación con lector de pantalla siguen necesitando a una
 * persona. Se dice aquí para que el informe verde no se lea como más de lo que
 * es.
 */

// Etiquetas de axe que corresponden a WCAG 2.2 nivel A y AA, que es lo que
// exige el enunciado. Se enumeran en vez de auditarlo todo para no mezclar
// buenas prácticas opcionales con incumplimientos del criterio.
const REGLAS = ["wcag2a", "wcag2aa", "wcag21a", "wcag21aa", "wcag22aa"];

async function auditar(page: import("@playwright/test").Page, nombre: string) {
  const resultado = await new AxeBuilder({ page }).withTags(REGLAS).analyze();

  // El informe se adjunta siempre, pase o falle: es la evidencia que pide la
  // sección 10.1, y un informe que solo existe cuando algo se rompe no sirve
  // para acreditar que no se rompió.
  await test.info().attach(`axe-${nombre}.json`, {
    body: JSON.stringify(resultado.violations, null, 2),
    contentType: "application/json",
  });

  const criticas = resultado.violations.map(
    (v) => `${v.impact ?? "sin impacto"} · ${v.id}: ${v.help} (${v.nodes.length} nodo/s)`,
  );
  expect(criticas, `accesibilidad en ${nombre}`).toEqual([]);
}

test.describe("Auditoría de accesibilidad (WCAG 2.2 AA)", () => {
  test("catálogo, registro y acceso: las pantallas públicas", async ({ page }) => {
    await page.goto("/");
    await expect(page.getByRole("heading", { level: 1 })).toBeVisible();
    await auditar(page, "catalogo");

    await page.goto("/login");
    await auditar(page, "login");

    await page.goto("/registro");
    await auditar(page, "registro");

    await page.goto("/restablecer-contrasena");
    await auditar(page, "restablecer-contrasena");
  });

  test("las pantallas del estudiante", async ({ page, request }) => {
    const admin = credencialesDeAdmin();
    const tokenProfesor = await tokenDeSesion(request, admin.email, admin.password);
    const curso = await publicarCursoDePrueba(request, tokenProfesor);

    const estudiante = await registrarEstudiante(request, sufijo());
    const token = await tokenDeSesion(request, estudiante.email, estudiante.password);
    await request.post(`${API}/api/v1/enrollments`, {
      headers: conToken(token),
      data: { course_id: curso.courseId },
    });

    await entrarPorLaInterfaz(page, estudiante);

    await page.goto("/mis-cursos");
    await expect(page.getByRole("heading", { level: 1 })).toBeVisible();
    await auditar(page, "mis-cursos");

    await page.goto(`/cursos/${curso.courseId}`);
    await expect(page.getByRole("heading", { level: 1 })).toBeVisible();
    await auditar(page, "detalle-de-curso");

    await page.goto(`/cursos/${curso.courseId}/recursos/${curso.recursoTexto}`);
    await expect(page.getByRole("heading", { level: 1 })).toBeVisible();
    await auditar(page, "recurso-de-texto");

    await page.goto(`/cursos/${curso.courseId}/recursos/${curso.recursoQuiz}`);
    await expect(page.getByRole("radio").first()).toBeVisible();
    await auditar(page, "quiz");

    await page.goto("/mis-insignias");
    await expect(page.getByRole("heading", { level: 1 })).toBeVisible();
    await auditar(page, "mis-insignias");

    await page.goto("/cuenta/sesiones");
    await expect(page.getByRole("heading", { level: 1 })).toBeVisible();
    await auditar(page, "sesiones");
  });

  test("las pantallas de autoría y administración", async ({ page, request }) => {
    const admin = credencialesDeAdmin();
    const token = await tokenDeSesion(request, admin.email, admin.password);
    const curso = await publicarCursoDePrueba(request, token);

    await entrarPorLaInterfaz(page, {
      email: admin.email,
      password: admin.password,
      fullName: "Administrador",
    });

    await page.goto("/profesor");
    await expect(page.getByRole("heading", { level: 1 })).toBeVisible();
    await auditar(page, "autoria");

    await page.goto(`/profesor/versiones/${curso.versionId}`);
    await expect(page.getByRole("heading", { level: 1 })).toBeVisible();
    await auditar(page, "editor-de-version");

    await page.goto(`/profesor/versiones/${curso.versionId}/previsualizacion`);
    await expect(page.getByRole("heading", { level: 1 })).toBeVisible();
    await auditar(page, "previsualizacion");

    await page.goto(`/profesor/versiones/${curso.versionId}/quiz/${curso.recursoQuiz}`);
    await expect(page.getByRole("heading", { level: 1 })).toBeVisible();
    await auditar(page, "autoria-de-quiz");

    await page.goto("/admin");
    await expect(page.getByRole("heading", { level: 1 })).toBeVisible();
    await auditar(page, "administracion");
  });

  test("la interfaz en inglés se audita igual que en español", async ({ page, request }) => {
    // Traducir puede romper la accesibilidad de formas que no se ven en el
    // idioma de origen: una etiqueta que se queda sin traducir deja al lector
    // de pantalla anunciando en un idioma y leyendo en otro.
    const estudiante = await registrarEstudiante(request, sufijo());
    await entrarPorLaInterfaz(page, estudiante);
    await page.getByLabel(/idioma|language/i).selectOption("en");
    await expect(page.locator("html")).toHaveAttribute("lang", "en");
    await auditar(page, "catalogo-en");
  });
});
