# Prueba de carga — Etapa 1

El enunciado pone esta prueba como condición de aceptación: «la prueba de carga
de Etapa 1 y la auditoría automática de accesibilidad no pueden presentar
incumplimientos críticos» (sección 10). Y en el segmento 9 pide «p95,
escalamiento, observabilidad, fallos, backup, restauración y RTO/RPO».

Lo que hay aquí cubre el p95 y el escalamiento. Lo demás sigue pendiente y está
listado al final, sin disfrazarlo.

## Qué es «Etapa 1»

El enunciado fija el objetivo del sistema —50.000 usuarios registrados y 2.000
concurrentes— pero **no da un p95**. Los umbrales de abajo los fijamos nosotros,
que es lo que pide «límites de rendimiento documentados».

Etapa 1 es el primer escalón hacia ese objetivo, medible en una máquina de
desarrollo: **200 usuarios virtuales sostenidos durante 3 minutos**, repartidos
según cómo se comporta de verdad un MOOC.

| Escenario | Peso | Por qué ese peso |
|---|---|---|
| Catálogo | 30 % | Es la primera pantalla y la única que ve quien no se ha registrado |
| Consumo de contenido | 55 % | Es a lo que se dedica un estudiante inscrito: leer y avanzar |
| Quiz | 15 % | Se presenta pocas veces por curso, pero es lo más caro por petición |
| Login | 8/min | A ritmo fijo, dentro del presupuesto del limitador |

Los 2.000 concurrentes se alcanzan subiendo `VUS_OBJETIVO` y escalando la API
(`--scale api=N`). Esta prueba es el instrumento; el escalón se elige por fuera.

## Umbrales

Son la definición operativa de «sin incumplimientos críticos». k6 sale con
código 99 si alguno se rompe, así que el CI puede colgar de ellos.

| Métrica | Umbral | Razón |
|---|---|---|
| Tasa de error global | < 0,5 % | Un error cada doscientas peticiones es el límite de lo tolerable |
| p95 catálogo | < 400 ms | Lectura sin sesión y cacheable: lo más barato que hace la plataforma |
| p99 catálogo | < 800 ms | |
| p95 consumo | < 600 ms | Resuelve inscripción, autoriza el recurso y firma una URL |
| p99 consumo | < 1200 ms | |
| p95 quiz | < 900 ms | Escribe en cada respuesta y califica al enviar |
| p99 quiz | < 1800 ms | |
| p95 login | < 2000 ms | Argon2id está calibrado para costar; medirlo con el rasero de una lectura invitaría a debilitarlo |
| Progreso aceptado | > 99 % | Mide que el avance se contabilizó, no que respondió rápido |
| Login limitado (429) | < 1 % | Si el limitador dispara a 8/min, está mal calibrado |
| Comprobaciones | > 99 % | Incluye las de comportamiento, no solo las de estado |

Dos de las comprobaciones no son de rendimiento y están aquí a propósito,
porque son condiciones verificables del enunciado que solo se romperían bajo
concurrencia:

- **La clave correcta nunca llega al cliente.** Se comprueba en cada intento.
- **El envío del intento es idempotente.** Cada intento se envía dos veces con
  la misma `Idempotency-Key` y se exige la misma nota.

## Cómo se ejecuta

```bash
cp .env.example .env
docker compose up -d --build

# Datos sintéticos: 500 cuentas, un curso publicado de 27 recursos, sesiones.
docker compose --profile carga run --rm seed

# La prueba. Escribe load/salida/resumen-etapa1.json como evidencia.
docker compose --profile carga run --rm k6
```

Para el escalón siguiente, sin tocar el guion:

```bash
docker compose up -d --scale api=3
VUS_OBJETIVO=600 MESETA=5m docker compose --profile carga run --rm k6
```

Sembrar es idempotente: repetir `seed` reutiliza el curso y las cuentas que ya
existan, así que repetir la prueba no multiplica datos.

## Dos decisiones que conviene entender antes de leer los números

**Las sesiones se siembran, no se obtienen por el endpoint de login.** El
limitador de tasa del login es de diez intentos por minuto **y por IP**, y todo
el tráfico de k6 sale de una sola IP. Autenticar 500 cuentas por la puerta
principal tardaría casi una hora y mediría el limitador, no la plataforma.
Sembrar las sesiones deja el control de seguridad intacto —la prueba lo
ejercita aparte, en su propio escenario y dentro de su presupuesto— y permite
medir lo que importa. No se desactiva nada para que la prueba pase.

**La prueba no mide la carga multimedia ni la transcodificación.** Eso vive en
el worker y en FFmpeg, tiene su propio segmento en la demostración, y meterlo
aquí convertiría el p95 de la API en el p95 de FFmpeg. El curso sintético es
todo texto y quizzes justamente por eso.

## Lo que esta prueba todavía no acredita

El segmento 9 pide más de lo que hay aquí. Queda pendiente, y no se puede
presentar como cubierto:

- **Observabilidad**: hoy hay logs estructurados y correlación por
  `X-Request-Id`; faltan métricas y trazas (OpenTelemetry) que permitan ver
  *dónde* se va el p95 cuando se rompe un umbral.
- **Fallos inyectados** durante la carga: tirar una réplica de la API a mitad
  de la meseta y demostrar que el p95 se recupera.
- **Backup y restauración** con RPO ≤ 15 min y RTO ≤ 4 h, y la prueba de
  recuperación que los acredita.
- **Los resultados**. Esta prueba está escrita y verificada de forma estática,
  pero **no se ha ejecutado todavía**: el entorno donde se preparó no tiene
  demonio de Docker. La tabla de umbrales es la hipótesis a contrastar; los
  números reales y sus desviaciones van en este mismo archivo tras la primera
  corrida.
