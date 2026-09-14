// Prueba de carga de Etapa 1 (sección 10, segmento 9 del enunciado).
//
// La condición de aceptación dice que esta prueba "no puede presentar
// incumplimientos críticos". Aquí eso no es una frase: los umbrales de la
// sección `thresholds` son la definición operativa, y k6 sale con código 99
// si alguno se incumple. Un informe verde que no pueda fallar no acredita
// nada, así que los umbrales están puestos para poder fallar.
//
// Qué es "Etapa 1". El enunciado fija 50.000 usuarios registrados y 2.000
// concurrentes como objetivo del sistema, pero no da un p95. Etapa 1 es el
// primer escalón hacia ese objetivo, medible en una máquina de desarrollo:
// 200 usuarios virtuales sostenidos. El escalón se sube con VUS_OBJETIVO sin
// tocar el guion, y la extrapolación se documenta en el README junto a los
// resultados, que es donde el enunciado pide "límites de rendimiento
// documentados".
//
// Lo que NO mide: la carga multimedia y la transcodificación. Esas viven en
// el worker y en FFmpeg, tienen su propio segmento en la demostración, y
// mezclarlas aquí convertiría el p95 de la API en el p95 de FFmpeg.

import http from "k6/http";
import { check, group, sleep } from "k6";
import { Counter, Rate, Trend } from "k6/metrics";
import { SharedArray } from "k6/data";
import exec from "k6/execution";

const BASE = __ENV.BASE_URL || "http://localhost:8080";
const ESCENARIO = __ENV.ESCENARIO || "/salida/escenario.json";
const VUS = Number(__ENV.VUS_OBJETIVO || 200);
const MESETA = __ENV.MESETA || "3m";

// El archivo lo produce `cmd/seed`. SharedArray lo carga una vez y lo comparte
// entre todos los VU: cargarlo por VU multiplicaría por 200 la memoria y el
// arranque, y falsearía la medición del primer minuto.
const datos = new SharedArray("escenario", () => [JSON.parse(open(ESCENARIO))]);
const escenario = datos[0];

// Métricas propias. Las de k6 miden HTTP; estas miden la plataforma: si el
// progreso se contabilizó, si el intento se calificó, si el limitador de tasa
// hizo su trabajo.
const progresoAceptado = new Rate("mooc_progreso_aceptado");
const intentosCalificados = new Counter("mooc_intentos_calificados");
const notaDelIntento = new Trend("mooc_nota_intento");
const loginLimitado = new Rate("mooc_login_limitado");

export const options = {
  // Descartar la primera respuesta de cada VU evitaría medir el arranque en
  // frío, pero ese arranque es parte de lo que un usuario real sufre. Se
  // mide, y se le da al sistema una rampa para calentar antes de la meseta.
  scenarios: {
    // 1. Navegación del catálogo: sin sesión, es la primera pantalla y la
    //    única que ve alguien que todavía no se registró.
    catalogo: {
      executor: "ramping-vus",
      exec: "navegarCatalogo",
      startVUs: 0,
      stages: [
        { duration: "30s", target: Math.ceil(VUS * 0.3) },
        { duration: MESETA, target: Math.ceil(VUS * 0.3) },
        { duration: "30s", target: 0 },
      ],
      tags: { escenario: "catalogo" },
    },

    // 2. Consumo de contenido: el grueso del tráfico de un MOOC. Abrir un
    //    recurso, leerlo y emitir los heartbeats de progreso.
    consumo: {
      executor: "ramping-vus",
      exec: "consumirContenido",
      startVUs: 0,
      stages: [
        { duration: "30s", target: Math.ceil(VUS * 0.55) },
        { duration: MESETA, target: Math.ceil(VUS * 0.55) },
        { duration: "30s", target: 0 },
      ],
      tags: { escenario: "consumo" },
    },

    // 3. Quizzes: lo más caro de la plataforma, porque cada guardado y cada
    //    envío escriben y el envío califica en servidor.
    quiz: {
      executor: "ramping-vus",
      exec: "presentarQuiz",
      startVUs: 0,
      stages: [
        { duration: "30s", target: Math.ceil(VUS * 0.15) },
        { duration: MESETA, target: Math.ceil(VUS * 0.15) },
        { duration: "30s", target: 0 },
      ],
      tags: { escenario: "quiz" },
    },

    // 4. Login por la puerta principal, a ritmo constante y dentro del
    //    presupuesto del limitador (10/min por IP). No busca romper nada:
    //    busca medir el coste real de verificar una contraseña con Argon2,
    //    que es deliberadamente lento y no debe esconderse detrás de las
    //    sesiones sembradas.
    login: {
      executor: "constant-arrival-rate",
      exec: "iniciarSesion",
      rate: 8,
      timeUnit: "1m",
      duration: MESETA,
      preAllocatedVUs: 2,
      tags: { escenario: "login" },
    },
  },

  thresholds: {
    // Umbral global. Un error cada doscientas peticiones bajo carga sostenida
    // es el límite de lo tolerable; por encima, algo se está cayendo.
    http_req_failed: [{ threshold: "rate<0.005", abortOnFail: false }],

    // Lectura de catálogo: sin sesión y cacheable, es lo más barato que hace
    // la plataforma. Si esto se degrada, el problema es de infraestructura.
    "http_req_duration{escenario:catalogo}": ["p(95)<400", "p(99)<800"],

    // Consumo: resuelve inscripción, autoriza el recurso y firma una URL.
    "http_req_duration{escenario:consumo}": ["p(95)<600", "p(99)<1200"],

    // Quiz: escribe en cada respuesta y califica al enviar. Se le concede más
    // porque hace más, no porque importe menos.
    "http_req_duration{escenario:quiz}": ["p(95)<900", "p(99)<1800"],

    // El login es el único que puede pasar del segundo sin que sea un fallo:
    // Argon2id está calibrado para costar. Medirlo con el mismo rasero que
    // una lectura invitaría a debilitarlo, que es exactamente al revés.
    "http_req_duration{escenario:login}": ["p(95)<2000"],

    // Umbrales de comportamiento, no de latencia. Son los que distinguen
    // "respondió rápido" de "hizo lo que debía".
    mooc_progreso_aceptado: ["rate>0.99"],
    mooc_login_limitado: ["rate<0.01"],
    checks: ["rate>0.99"],
  },
};

function sesion(indice) {
  const cuentas = escenario.estudiantes;
  return cuentas[indice % cuentas.length];
}

function auth(token) {
  // Cabecera Authorization y no cookie: el middleware CSRF exige el doble
  // envío solo cuando la sesión viaja en cookie, y un generador de carga no
  // es un navegador. Medir el CSRF aquí mediría el guion, no la plataforma;
  // su cobertura está en las pruebas de integración.
  return {
    headers: { Authorization: `Bearer ${token}`, "Content-Type": "application/json" },
  };
}

export function navegarCatalogo() {
  group("catálogo", () => {
    const lista = http.get(`${BASE}/api/v1/catalog`, { tags: { escenario: "catalogo" } });
    check(lista, {
      "catálogo responde 200": (r) => r.status === 200,
      "catálogo trae cursos": (r) => (r.json("items") || []).length > 0,
    });

    // Una búsqueda filtrada: es el camino que no puede servirse de una caché
    // trivial, y por tanto el que de verdad mide la consulta.
    const filtrado = http.get(`${BASE}/api/v1/catalog?q=cloud&level=Introductorio`, {
      tags: { escenario: "catalogo" },
    });
    check(filtrado, { "búsqueda responde 200": (r) => r.status === 200 });

    const detalle = http.get(`${BASE}/api/v1/catalog/${escenario.course_id}`, {
      tags: { escenario: "catalogo" },
    });
    check(detalle, { "detalle responde 200": (r) => r.status === 200 });
  });
  sleep(1 + Math.random() * 2);
}

export function consumirContenido() {
  const cuenta = sesion(exec.vu.idInTest);
  const cfg = auth(cuenta.token);
  const recursos = escenario.recursos_texto;
  const recurso = recursos[Math.floor(Math.random() * recursos.length)];

  group("consumo", () => {
    const contenido = http.get(`${BASE}/api/v1/resources/${recurso}/content`, {
      ...cfg,
      tags: { escenario: "consumo" },
    });
    check(contenido, { "contenido responde 200": (r) => r.status === 200 });

    // Apertura y permanencia: el progreso lo decide el servidor a partir de
    // estas señales, así que la prueba las emite como las emitiría el
    // navegador, no un porcentaje inventado por el cliente.
    const apertura = http.post(
      `${BASE}/api/v1/resources/${recurso}/progress`,
      JSON.stringify({ type: "open", complete: false }),
      { ...cfg, tags: { escenario: "consumo" } },
    );
    progresoAceptado.add(apertura.status === 200);
    check(apertura, { "apertura registrada": (r) => r.status === 200 });

    sleep(2 + Math.random() * 3);

    const latido = http.post(
      `${BASE}/api/v1/resources/${recurso}/progress`,
      JSON.stringify({ type: "heartbeat", complete: false }),
      { ...cfg, tags: { escenario: "consumo" } },
    );
    progresoAceptado.add(latido.status === 200);

    const resumen = http.get(`${BASE}/api/v1/enrollments/${escenario.course_id}/progress`, {
      ...cfg,
      tags: { escenario: "consumo" },
    });
    check(resumen, { "resumen de progreso responde 200": (r) => r.status === 200 });
  });
  sleep(1 + Math.random() * 2);
}

export function presentarQuiz() {
  const cuenta = sesion(exec.vu.idInTest);
  const cfg = auth(cuenta.token);
  const quizzes = escenario.recursos_quiz;
  const recurso = quizzes[Math.floor(Math.random() * quizzes.length)];

  group("quiz", () => {
    const inicio = http.post(`${BASE}/api/v1/resources/${recurso}/quiz/attempts`, null, {
      ...cfg,
      tags: { escenario: "quiz" },
    });
    if (!check(inicio, { "intento abierto": (r) => r.status === 200 || r.status === 201 })) {
      return;
    }

    const intento = inicio.json();
    const attemptId = intento.attempt_id;
    const preguntas = intento.questions || [];

    // Que el cliente no reciba la clave correcta es una condición verificable
    // del enunciado, no un detalle: se comprueba en cada intento, bajo carga,
    // porque una fuga que solo aparece con concurrencia seguiría siendo fuga.
    check(inicio, {
      "el intento no expone la respuesta correcta": (r) =>
        !/"is_correct"|"es_correcta"/.test(r.body || ""),
    });

    for (const p of preguntas) {
      const opciones = p.options || [];
      if (opciones.length === 0) continue;
      // Se elige al azar: así el escenario produce aprobados y reprobados, y
      // el cálculo de la nota recorre las dos ramas.
      const elegida = opciones[Math.floor(Math.random() * opciones.length)];
      const guardado = http.put(
        `${BASE}/api/v1/quiz/attempts/${attemptId}/answers`,
        JSON.stringify({
          question_stable_id: p.stable_id,
          selected_option_stable_ids: [elegida.stable_id],
        }),
        { ...cfg, tags: { escenario: "quiz" } },
      );
      check(guardado, { "respuesta guardada": (r) => r.status === 200 || r.status === 204 });
      sleep(0.5 + Math.random());
    }

    // Clave de idempotencia estable por intento: reenviar no debe volver a
    // calificar. Es una condición verificable del enunciado y aquí se ejerce
    // bajo concurrencia, que es cuando de verdad se rompería.
    const cabeceras = {
      ...cfg.headers,
      "Idempotency-Key": `carga-${attemptId}`,
    };
    const envio = http.post(`${BASE}/api/v1/quiz/attempts/${attemptId}/submit`, null, {
      headers: cabeceras,
      tags: { escenario: "quiz" },
    });
    if (check(envio, { "intento calificado": (r) => r.status === 200 })) {
      intentosCalificados.add(1);
      const nota = envio.json("score");
      if (typeof nota === "number") notaDelIntento.add(nota);

      const reenvio = http.post(`${BASE}/api/v1/quiz/attempts/${attemptId}/submit`, null, {
        headers: cabeceras,
        tags: { escenario: "quiz" },
      });
      check(reenvio, {
        "el reenvío devuelve la misma nota": (r) => r.status === 200 && r.json("score") === nota,
      });
    }
  });
  sleep(2 + Math.random() * 3);
}

export function iniciarSesion() {
  const cuenta = escenario.cuenta_de_prueba;
  const res = http.post(
    `${BASE}/api/v1/auth/login`,
    JSON.stringify({ email: cuenta.email, password: cuenta.password }),
    { headers: { "Content-Type": "application/json" }, tags: { escenario: "login" } },
  );
  loginLimitado.add(res.status === 429);
  check(res, { "login responde 200": (r) => r.status === 200 });
}

// El resumen se escribe también como JSON para que quede como evidencia
// adjuntable a la demostración: un p95 leído en una terminal que ya se cerró
// no acredita nada.
export function handleSummary(data) {
  return {
    stdout: resumenLegible(data),
    "/salida/resumen-etapa1.json": JSON.stringify(data, null, 2),
  };
}

function resumenLegible(data) {
  const m = data.metrics;
  const p95 = (nombre) => {
    const v = m[nombre];
    return v && v.values ? `${v.values["p(95)"]?.toFixed(0) ?? "—"} ms` : "—";
  };
  const lineas = [
    "",
    "Prueba de carga — Etapa 1",
    "=========================",
    `VU objetivo:        ${VUS}`,
    `Peticiones:         ${m.http_reqs?.values?.count ?? 0}`,
    `Tasa de error:      ${((m.http_req_failed?.values?.rate ?? 0) * 100).toFixed(3)} %`,
    `p95 catálogo:       ${p95("http_req_duration{escenario:catalogo}")}`,
    `p95 consumo:        ${p95("http_req_duration{escenario:consumo}")}`,
    `p95 quiz:           ${p95("http_req_duration{escenario:quiz}")}`,
    `p95 login:          ${p95("http_req_duration{escenario:login}")}`,
    `Intentos calificados: ${m.mooc_intentos_calificados?.values?.count ?? 0}`,
    `Comprobaciones OK:  ${((m.checks?.values?.rate ?? 0) * 100).toFixed(2)} %`,
    "",
  ];
  return lineas.join("\n");
}
