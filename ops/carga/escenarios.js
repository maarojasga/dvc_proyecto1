import http from "k6/http";
import { check, group, sleep } from "k6";
import { Counter, Rate, Trend } from "k6/metrics";

/**
 * Prueba de carga de la Plataforma MOOC.
 *
 * Los objetivos del enunciado son 50.000 usuarios registrados y 2.000
 * concurrentes. Esta prueba se ejecuta por etapas —ver ops/carga/README.md— y
 * los umbrales son los mismos en todas: lo que cambia es la carga, no lo que
 * se considera aceptable.
 *
 * La mezcla de tráfico no es uniforme a propósito. En un MOOC la mayoría de
 * las peticiones son lectura de catálogo y de contenido, y una fracción
 * pequeña son escrituras (progreso, evaluaciones). Medir con un reparto
 * uniforme daría un número que no corresponde a nada.
 */

/**
 * randomIntBetween va aquí en lugar de importarse de jslib: una prueba de
 * carga que necesita internet para arrancar no se puede ejecutar en una red
 * cerrada, y esto son dos líneas.
 */
function randomIntBetween(min, max) {
  return Math.floor(Math.random() * (max - min + 1)) + min;
}

/**
 * Qué cuenta como petición fallida.
 *
 * Por omisión k6 marca como fallo todo lo que no sea 2xx, y eso aquí da un
 * número falso: el 304 del catálogo es precisamente el acierto que se quiere
 * medir, no un error. Se declaran esperados los 2xx y el 304.
 *
 * Los 4xx quedan fuera de "esperado" a propósito: un 401 o un 429 durante la
 * prueba sí son incumplimientos —el sistema está rechazando tráfico legítimo—
 * y tienen que aparecer.
 */
http.setResponseCallback(http.expectedStatuses({ min: 200, max: 204 }, 304));

const API = __ENV.CARGA_API ?? "http://localhost:8080";
const PATRON_CORREO = __ENV.CARGA_CORREO_PATRON ?? "estudiante%d@carga.local";
const CLAVE = __ENV.CARGA_CLAVE ?? "CargaDePrueba123!";
const ESTUDIANTES = Number(__ENV.CARGA_ESTUDIANTES ?? 50000);

/** ETAPA elige el perfil de carga; ver README. */
const ETAPA = __ENV.CARGA_ETAPA ?? "1";

// --- Métricas propias -------------------------------------------------------

// Se separan por flujo porque un p95 global esconde lo que importa: el
// catálogo es rápido y el inicio de sesión es lento por diseño (bcrypt), y
// mezclarlos hace que el número no signifique nada.
const latenciaCatalogo = new Trend("mooc_catalogo_ms", true);
const latenciaCurso = new Trend("mooc_curso_ms", true);
const latenciaContenido = new Trend("mooc_contenido_ms", true);
const latenciaProgreso = new Trend("mooc_progreso_ms", true);
const latenciaLogin = new Trend("mooc_login_ms", true);

const aciertosCache = new Rate("mooc_catalogo_304");
const erroresDeNegocio = new Counter("mooc_errores_negocio");
// Cuántas veces hubo que renovar una sesión. Un número alto indica que las
// sesiones se están invalidando solas, que sería un fallo del sistema; uno
// bajo y estable es el artefacto conocido del generador de carga.
const sesionesRenovadas = new Counter("mooc_sesiones_renovadas");

// --- Umbrales ---------------------------------------------------------------

/**
 * Los umbrales son la definición de "sin incumplimientos críticos". Están
 * separados por flujo y justificados:
 *
 *  - Catálogo y curso: lectura cacheable y con índice. 500 ms de p95 es
 *    holgado para una consulta paginada por cursor.
 *  - Contenido: incluye firmar una URL en el almacén de objetos, que añade una
 *    ida y vuelta. 800 ms.
 *  - Progreso: escritura pequeña, pero con recálculo del avance del curso.
 *    800 ms.
 *  - Inicio de sesión: bcrypt con coste 12 tarda ~250 ms *a propósito*. Poner
 *    aquí el mismo umbral que en una lectura sería pedir que el hash fuera
 *    débil. 1.500 ms.
 *
 * La tasa de error se mide sobre fallos de verdad (5xx y caídas), no sobre los
 * 4xx: un 404 al pedir un curso ajeno es el sistema funcionando.
 */
export const options = {
  thresholds: {
    "http_req_failed{esperado:si}": ["rate<0.001"],
    mooc_errores_negocio: ["count<1"],
    // Renovar una sesión es legítimo, pero si pasa mucho significa que las
    // sesiones se invalidan solas y el flujo estaría midiendo bcrypt en lugar
    // del consumo de contenido.
    mooc_sesiones_renovadas: ["count<5"],

    "mooc_catalogo_ms": ["p(95)<500"],
    "mooc_curso_ms": ["p(95)<500"],
    "mooc_contenido_ms": ["p(95)<800"],
    "mooc_progreso_ms": ["p(95)<800"],
    "mooc_login_ms": ["p(95)<1500"],

    // Disponibilidad: el 99,9 % de las peticiones tiene que completarse.
    checks: ["rate>0.999"],
  },
  scenarios: escenariosDeLaEtapa(ETAPA),
  // El tarro de cookies persiste entre iteraciones del mismo usuario virtual.
  // Por omisión k6 lo reinicia en cada iteración, y con eso la sesión
  // guardada se queda sin su cookie: cada iteración daba un 401 y volvía a
  // autenticarse, así que el flujo medía bcrypt en lugar del consumo de
  // contenido. Un navegador conserva la sesión entre páginas, que es lo que se
  // quiere representar.
  noCookiesReset: true,
  discardResponseBodies: false,
  summaryTrendStats: ["avg", "min", "med", "p(90)", "p(95)", "p(99)", "max"],
};

/**
 * escenariosDeLaEtapa reparte la concurrencia objetivo entre los flujos.
 *
 * El reparto (70 % catálogo y curso, 20 % contenido y progreso, 10 %
 * identidad) sale de cómo se usa un MOOC: mucha gente mirando, bastante menos
 * consumiendo, y una minoría entrando en cada momento.
 */
function escenariosDeLaEtapa(etapa) {
  const objetivo = { "1": 200, "2": 1000, "3": 2000, humo: 10 }[etapa];
  if (!objetivo) {
    throw new Error(`etapa desconocida: ${etapa} (válidas: humo, 1, 2, 3)`);
  }
  const duracion = __ENV.CARGA_DURACION ?? (etapa === "humo" ? "30s" : "5m");
  const subida = __ENV.CARGA_SUBIDA ?? (etapa === "humo" ? "5s" : "1m");

  const reparto = (fraccion) => Math.max(1, Math.round(objetivo * fraccion));

  const comun = (fraccion, exec) => ({
    executor: "ramping-vus",
    exec,
    startVUs: 0,
    // La subida gradual no es cosmética: arrancar 2.000 usuarios de golpe mide
    // el arranque en frío (pool de conexiones vacío, cachés frías) en lugar
    // del estado estacionario, que es lo que se quiere conocer.
    stages: [
      { duration: subida, target: reparto(fraccion) },
      { duration: duracion, target: reparto(fraccion) },
      { duration: "30s", target: 0 },
    ],
    gracefulRampDown: "30s",
  });

  const todos = {
    navegarCatalogo: comun(0.7, "navegarCatalogo"),
    consumirContenido: comun(0.2, "consumirContenido"),
    identidad: comun(0.1, "identidad"),
  };

  // CARGA_ESCENARIOS permite ejecutar un subconjunto, separado por comas.
  // Sirve para atribuir una degradación: si el p95 del catálogo se recupera al
  // quitar el escenario de identidad, el cuello está en el coste de bcrypt y
  // no en la lectura.
  const filtro = __ENV.CARGA_ESCENARIOS;
  if (!filtro) return todos;

  const pedidos = filtro.split(",").map((x) => x.trim()).filter(Boolean);
  const desconocidos = pedidos.filter((x) => !(x in todos));
  if (desconocidos.length) {
    throw new Error(`escenarios desconocidos: ${desconocidos.join(", ")} (válidos: ${Object.keys(todos).join(", ")})`);
  }
  return Object.fromEntries(pedidos.map((x) => [x, todos[x]]));
}

// --- Utilidades -------------------------------------------------------------

/** correoDeUsuario reparte las cuentas sembradas entre los usuarios virtuales. */
function correoDeUsuario() {
  // __VU es 1..N y se reutiliza entre escenarios, así que se mezcla con el
  // número de iteración para que dos usuarios virtuales no compitan por la
  // misma cuenta y se invaliden las sesiones mutuamente.
  const n = (__VU * 7919 + __ITER) % ESTUDIANTES;
  return PATRON_CORREO.replace("%d", String(n));
}

/**
 * entrar inicia sesión y devuelve las cabeceras con el token anti-CSRF.
 *
 * La sesión se guarda por usuario virtual: iniciar sesión en cada iteración
 * mediría bcrypt y no el flujo que se quiere medir.
 */
function entrar(jar) {
  const res = http.post(
    `${API}/api/v1/auth/login`,
    JSON.stringify({ email: correoDeUsuario(), password: CLAVE }),
    { headers: { "Content-Type": "application/json" }, jar, tags: { flujo: "login", esperado: "si" } },
  );
  latenciaLogin.add(res.timings.duration);
  const bien = check(res, { "el inicio de sesión responde 200": (r) => r.status === 200 });
  if (!bien) {
    erroresDeNegocio.add(1);
    return null;
  }
  // El token anti-CSRF se lee de la cookie y se repite en la cabecera, igual
  // que hace el navegador.
  const csrf = res.cookies["mooc_csrf"]?.[0]?.value;
  return { "Content-Type": "application/json", "X-CSRF-Token": csrf ?? "" };
}

/** pensar simula el tiempo entre acciones de una persona. */
function pensar(min, max) {
  sleep(randomIntBetween(min, max) / 1000);
}

// --- Flujo: navegar el catálogo ---------------------------------------------

/**
 * navegarCatalogo es el flujo más frecuente: alguien mira la lista, pasa de
 * página y abre un curso. Va sin sesión, porque el catálogo es público.
 *
 * Reenvía el ETag en la segunda pasada, que es lo que hace un navegador, y
 * mide cuántas respuestas son 304: si ese número es bajo, el ETag no está
 * ahorrando nada y conviene saberlo.
 */
export function navegarCatalogo() {
  group("catálogo", () => {
    const primera = http.get(`${API}/api/v1/catalog?limit=20`, {
      tags: { flujo: "catalogo", esperado: "si" },
    });
    latenciaCatalogo.add(primera.timings.duration);
    if (!check(primera, { "el catálogo responde 200": (r) => r.status === 200 })) {
      erroresDeNegocio.add(1);
      return;
    }

    const etag = primera.headers["Etag"];
    let cuerpo;
    try {
      cuerpo = primera.json();
    } catch {
      erroresDeNegocio.add(1);
      return;
    }

    pensar(500, 2000);

    // Volver a la lista: el navegador reenvía el ETag.
    if (etag) {
      const repetida = http.get(`${API}/api/v1/catalog?limit=20`, {
        headers: { "If-None-Match": etag },
        tags: { flujo: "catalogo", esperado: "si" },
      });
      latenciaCatalogo.add(repetida.timings.duration);
      aciertosCache.add(repetida.status === 304);
      check(repetida, { "la relectura responde 200 o 304": (r) => r.status === 200 || r.status === 304 });
    }

    // Pasar de página con el cursor.
    if (cuerpo?.next_cursor) {
      const siguiente = http.get(
        `${API}/api/v1/catalog?limit=20&cursor=${encodeURIComponent(cuerpo.next_cursor)}`,
        { tags: { flujo: "catalogo", esperado: "si" } },
      );
      latenciaCatalogo.add(siguiente.timings.duration);
      check(siguiente, { "la página siguiente responde 200": (r) => r.status === 200 });
    }

    // Y abrir un curso de la lista.
    const items = cuerpo?.items ?? [];
    if (items.length) {
      const elegido = items[randomIntBetween(0, items.length - 1)];
      const cursoID = elegido.CourseID ?? elegido.course_id;
      if (cursoID) {
        pensar(300, 1500);
        const curso = http.get(`${API}/api/v1/catalog/${cursoID}`, {
          tags: { flujo: "curso", esperado: "si" },
        });
        latenciaCurso.add(curso.timings.duration);
        check(curso, { "el detalle del curso responde 200": (r) => r.status === 200 });
      }
    }
  });

  pensar(1000, 4000);
}

// --- Flujo: consumir contenido ----------------------------------------------

/**
 * consumirContenido es el flujo del estudiante inscrito: entra, abre uno de
 * sus cursos, abre un recurso y reporta avance.
 *
 * La sesión se reutiliza entre iteraciones del mismo usuario virtual. Sin eso,
 * el flujo estaría dominado por bcrypt y mediría el inicio de sesión con otro
 * nombre.
 */
export function consumirContenido() {
  group("consumo", () => {
    const mios = conSesion((h) =>
      http.get(`${API}/api/v1/enrollments/mine`, {
        headers: h,
        tags: { flujo: "inscripciones", esperado: "si" },
      }),
    );
    if (!mios) {
      erroresDeNegocio.add(1);
      return;
    }
    if (!check(mios, { "mis inscripciones responden 200": (r) => r.status === 200 })) {
      erroresDeNegocio.add(1);
      return;
    }
    const cabeceras = sesiones[__VU];
    let inscripciones = [];
    try {
      inscripciones = mios.json("items") ?? [];
    } catch {
      erroresDeNegocio.add(1);
      return;
    }
    if (!inscripciones.length) return;

    const inscripcion = inscripciones[randomIntBetween(0, inscripciones.length - 1)];
    const cursoID = inscripcion.CourseID ?? inscripcion.course_id;
    if (!cursoID) return;

    pensar(300, 1200);

    const curso = http.get(`${API}/api/v1/catalog/${cursoID}`, {
      tags: { flujo: "curso", esperado: "si" },
    });
    latenciaCurso.add(curso.timings.duration);
    if (!check(curso, { "el curso inscrito responde 200": (r) => r.status === 200 })) return;

    const recursoID = primerRecurso(curso);
    if (!recursoID) return;

    pensar(300, 1200);

    const contenido = http.get(`${API}/api/v1/resources/${recursoID}/content`, {
      headers: cabeceras,
      tags: { flujo: "contenido", esperado: "si" },
    });
    latenciaContenido.add(contenido.timings.duration);
    check(contenido, {
      "la entrega del recurso responde 200": (r) => r.status === 200,
    });

    // Señales de avance: abrir, permanecer y cerrar. Es lo que produce un
    // estudiante leyendo, y la escritura más frecuente del sistema.
    for (const tipo of ["open", "heartbeat"]) {
      const evento = http.post(
        `${API}/api/v1/recursos/${recursoID}/progreso`,
        JSON.stringify({ type: tipo, complete: false }),
        { headers: cabeceras, tags: { flujo: "progreso", esperado: "si" } },
      );
      latenciaProgreso.add(evento.timings.duration);
      check(evento, { "el progreso responde 200": (r) => r.status === 200 });
      pensar(500, 1500);
    }

    const cierre = http.post(
      `${API}/api/v1/recursos/${recursoID}/progreso`,
      JSON.stringify({ type: "close", complete: false }),
      { headers: cabeceras, tags: { flujo: "progreso", esperado: "si" } },
    );
    latenciaProgreso.add(cierre.timings.duration);
    check(cierre, { "el cierre responde 200": (r) => r.status === 200 });

    const avance = http.get(`${API}/api/v1/cursos/${cursoID}/progreso`, {
      headers: cabeceras,
      tags: { flujo: "progreso", esperado: "si" },
    });
    latenciaProgreso.add(avance.timings.duration);
    check(avance, { "el resumen de avance responde 200": (r) => r.status === 200 });
  });

  pensar(2000, 6000);
}

/** primerRecurso saca un recurso visible del árbol del curso. */
function primerRecurso(respuesta) {
  let arbol;
  try {
    arbol = respuesta.json();
  } catch {
    return null;
  }
  for (const modulo of arbol?.Modules ?? []) {
    for (const unidad of modulo?.Units ?? []) {
      for (const recurso of unidad?.Resources ?? []) {
        if (recurso?.Visible) return recurso.ID;
      }
    }
  }
  return null;
}

// sesiones guarda las cabeceras por usuario virtual. k6 crea un contexto de
// JavaScript por usuario virtual, así que este mapa no se comparte entre ellos
// y cada uno mantiene su propia sesión.
const sesiones = {};

function sesionDelUsuario() {
  if (sesiones[__VU] === undefined) {
    sesiones[__VU] = entrar();
  }
  return sesiones[__VU];
}

/**
 * conSesion ejecuta una petición autenticada y, si vuelve 401, se reautentica
 * y la repite una vez.
 *
 * Hace falta por dos razones y las dos son legítimas. La primera es que k6
 * reutiliza los números de usuario virtual entre escenarios: cuando el
 * escenario de identidad cierra su sesión, el tarro de cookies que queda puede
 * heredarlo otro escenario, y sin reintento eso cuenta como fallo del sistema
 * cuando es un artefacto del generador de carga. La segunda es que es lo que
 * hace un cliente de verdad: una sesión caducada se renueva, no rompe la
 * pantalla.
 *
 * El reintento es uno solo. Si tras reautenticarse sigue dando 401, eso ya es
 * un fallo real y tiene que contarse como tal.
 */
function conSesion(peticion) {
  let cabeceras = sesionDelUsuario();
  if (!cabeceras) return null;

  let res = peticion(cabeceras);
  if (res.status !== 401) return res;

  sesionesRenovadas.add(1);
  sesiones[__VU] = entrar();
  cabeceras = sesiones[__VU];
  if (!cabeceras) return res;
  return peticion(cabeceras);
}

// --- Flujo: identidad -------------------------------------------------------

/**
 * identidad mide el inicio de sesión y la lectura de la propia cuenta.
 *
 * Va en su propio escenario porque bcrypt domina su latencia y mezclarlo con
 * las lecturas desplazaría el p95 de todo lo demás.
 */
export function identidad() {
  // Tarro de cookies propio de esta iteración, por dos razones. Una:
  // representa a alguien que abre el navegador, entra y se va, que es el
  // flujo que se quiere medir. Dos, y más importante: k6 reparte los números
  // de usuario virtual entre escenarios, así que el cierre de sesión de aquí
  // limpiaba el tarro compartido y dejaba sin sesión al escenario de consumo.
  // Eso producía 401 que parecían fallos del sistema y eran del generador de
  // carga.
  const jar = new http.CookieJar();

  const cabeceras = entrar(jar);
  if (!cabeceras) {
    pensar(2000, 5000);
    return;
  }
  const yo = http.get(`${API}/api/v1/auth/me`, {
    headers: cabeceras,
    jar,
    tags: { flujo: "me", esperado: "si" },
  });
  check(yo, { "la propia cuenta responde 200": (r) => r.status === 200 });

  // Cerrar la sesión: si no, cada iteración deja una sesión abierta y la tabla
  // crece sin parar, lo que falsearía la medición al cabo de unos minutos.
  const salida = http.post(`${API}/api/v1/auth/logout`, null, {
    headers: cabeceras,
    jar,
    tags: { flujo: "logout", esperado: "si" },
  });
  check(salida, { "el cierre de sesión responde 200": (r) => r.status === 200 });

  pensar(3000, 8000);
}

// --- Resumen ----------------------------------------------------------------

export function handleSummary(datos) {
  const p95 = (nombre) => datos.metrics[nombre]?.values?.["p(95)"]?.toFixed(0) ?? "—";
  const lineas = [
    "",
    `Etapa ${ETAPA} — p95 por flujo (ms)`,
    `  catálogo        ${p95("mooc_catalogo_ms")}  (umbral 500)`,
    `  curso           ${p95("mooc_curso_ms")}  (umbral 500)`,
    `  contenido       ${p95("mooc_contenido_ms")}  (umbral 800)`,
    `  progreso        ${p95("mooc_progreso_ms")}  (umbral 800)`,
    `  inicio sesión   ${p95("mooc_login_ms")}  (umbral 1500)`,
    "",
    `  respuestas 304 del catálogo: ${((datos.metrics.mooc_catalogo_304?.values?.rate ?? 0) * 100).toFixed(1)} %`,
    `  errores de negocio: ${datos.metrics.mooc_errores_negocio?.values?.count ?? 0}`,
    "",
  ];
  return {
    stdout: lineas.join("\n"),
    // El JSON queda como evidencia reproducible de la ejecución.
    [`resultados/etapa-${ETAPA}.json`]: JSON.stringify(datos, null, 2),
  };
}
