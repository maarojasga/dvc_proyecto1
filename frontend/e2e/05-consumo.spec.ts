import { test, expect } from "@playwright/test";
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
 * Segmento 5 — Consumo de contenido.
 *
 * Criterio: multimedia, progreso y accesibilidad. Inscripción, entrega
 * autorizada del contenido y navegación por teclado.
 */
test.describe("5. Consumo de contenido", () => {
  test("el contenido solo se entrega a quien está inscrito", async ({ request }) => {
    const admin = credencialesDeAdmin();
    const tokenProfesor = await tokenDeSesion(request, admin.email, admin.password);
    const curso = await publicarCursoDePrueba(request, tokenProfesor);

    const estudiante = await registrarEstudiante(request, sufijo());
    const token = await tokenDeSesion(request, estudiante.email, estudiante.password);

    // Sin inscripción no hay contenido, aunque el curso sea público en el
    // catálogo y el identificador del recurso se conozca.
    const sinInscribir = await request.get(
      `${API}/api/v1/resources/${curso.recursoTexto}/content`,
      { headers: conToken(token) },
    );
    expect(sinInscribir.status(), "sin inscripción, el contenido no se entrega").toBe(404);

    const inscripcion = await request.post(`${API}/api/v1/enrollments`, {
      headers: conToken(token),
      data: { course_id: curso.courseId },
    });
    expect(inscripcion.ok()).toBeTruthy();

    const conInscripcion = await request.get(
      `${API}/api/v1/resources/${curso.recursoTexto}/content`,
      { headers: conToken(token) },
    );
    expect(conInscripcion.ok(), "ya inscrito, el contenido llega").toBeTruthy();
    expect((await conInscripcion.json()).markdown).toContain("Lección de apertura");
  });

  test("un estudiante se inscribe y abre la lección desde la interfaz", async ({
    page,
    request,
  }) => {
    const admin = credencialesDeAdmin();
    const tokenProfesor = await tokenDeSesion(request, admin.email, admin.password);
    const curso = await publicarCursoDePrueba(request, tokenProfesor);
    const estudiante = await registrarEstudiante(request, sufijo());

    await entrarPorLaInterfaz(page, estudiante);
    await page.goto(`/cursos/${curso.courseId}`);
    await expect(page.getByRole("heading", { name: curso.titulo })).toBeVisible();

    await page.getByRole("button", { name: /inscribirme|enroll/i }).click();
    await expect(page.getByRole("status")).toContainText(/inscripción|enrolled/i);

    await page.getByRole("link", { name: "Lección de apertura" }).click();
    await expect(page.getByRole("heading", { name: "Lección de apertura" })).toBeVisible();
    await expect(page.getByText("Texto de la lección")).toBeVisible();

    // El curso aparece en "Mis cursos" con su estado.
    await page.goto("/mis-cursos");
    await expect(page.getByRole("link", { name: /ver curso|view course/i }).first()).toBeVisible();
  });

  test("la navegación por teclado llega al contenido sin usar el ratón", async ({
    page,
    request,
  }) => {
    const estudiante = await registrarEstudiante(request, sufijo());
    await entrarPorLaInterfaz(page, estudiante);
    await page.goto("/");

    // El primer tabulador debe dar con el enlace de salto: es lo que permite
    // a quien navega con teclado esquivar el menú en cada página.
    await page.keyboard.press("Tab");
    const enfocado = page.locator(":focus");
    await expect(enfocado).toHaveText(/saltar al contenido|skip to the main content/i);

    await page.keyboard.press("Enter");
    await expect(page.locator("#main-content")).toBeVisible();
  });

  test("el cambio de idioma traduce la interfaz entera, no solo el menú", async ({
    page,
    request,
  }) => {
    const estudiante = await registrarEstudiante(request, sufijo());
    await entrarPorLaInterfaz(page, estudiante);

    await page.goto("/mis-cursos");
    await expect(page.getByRole("heading", { name: "Mis cursos" })).toBeVisible();

    await page.getByLabel(/idioma|language/i).selectOption("en");

    await expect(page.getByRole("heading", { name: "My courses" })).toBeVisible();
    await expect(
      page.getByRole("navigation").getByRole("link", { name: "Catalog", exact: true }),
    ).toBeVisible();
    // Y el atributo lang sigue al idioma: un lector de pantalla necesita saber
    // en qué idioma está el texto que va a pronunciar.
    await expect(page.locator("html")).toHaveAttribute("lang", "en");
  });
});
