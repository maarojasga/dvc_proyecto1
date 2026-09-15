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

La regla que no conviene saltarse al subir el escalón: **`SEED_STUDENTS` nunca
por debajo de `VUS_OBJETIVO`**. Con menos cuentas que usuarios virtuales, dos VU
comparten estudiante y se pisan el intento; `setup()` corta antes de empezar y
dice cuánto sembrar, pero es más rápido no provocarlo.

## Cómo leer el resultado

k6 sale con código 99 si rompe un umbral, así que lo primero es el código de
salida. Para lo demás, el resumen tiene cientos de líneas:

```bash
python3 load/leer-resumen.py load/salida/resumen-etapa1.json
```

```
escenario     p95 (ms)
catalogo           2.0
consumo            4.9
quiz               5.1
login            276.6

peticiones:     42075
errores:        0.000 %
comprobaciones: 44818 pasadas, 0 fallidas
umbrales rotos: ninguno

La corrida vale: los cuatro escenarios hicieron tráfico y ningún umbral se rompió.
```

Además de los umbrales comprueba **que los cuatro escenarios hicieron tráfico**,
que es justamente lo que aquel informe verde de más abajo no comprobaba: un
escenario que no llega a hacer una sola petición no aparece en el resumen, y su
ausencia es lo que hay que detectar. Sale con código 1 si algo falla.

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

## Resultados medidos

Primera corrida real, sobre la pila completa (Postgres 16, Redis 7, MinIO, API
y worker) en una máquina de desarrollo. El escenario tenía 200 cuentas
sembradas y un curso de 27 recursos.

| Métrica | Umbral | Medido a 200 VU | Margen |
|---|---|---|---|
| Peticiones | — | 42.075 en 4 min | — |
| Tasa de error | < 0,5 % | **0,000 %** | — |
| p95 catálogo | < 400 ms | **2 ms** | 200× |
| p95 consumo | < 600 ms | **5 ms** | 120× |
| p95 quiz | < 900 ms | **5 ms** | 180× |
| p95 login | < 2000 ms | **277 ms** | 7,2× |
| Comprobaciones | > 99 % | **100 %** | — |

Etapa 1 pasa sin acercarse a ningún umbral. Conviene leer eso con cuidado:
**los márgenes son tan amplios que los umbrales no están midiendo nada
todavía**. Sirven para detectar una regresión grosera, no para caracterizar el
sistema. El p95 del login es el único número informativo, y lo que mide es
Argon2id, que cuesta a propósito.

### Por qué setup() valida el escenario entero

Durante un tiempo esta prueba informó **0 % de error y 100 % de comprobaciones
con la mitad del tráfico sin ejecutar**, y eso es mucho peor que fallar.

El sembrador reutiliza el curso si ya existe, pero en esa rama cargaba la fila
de la versión sin su árbol de módulos, así que escribía `recursos_texto` y
`recursos_quiz` a `null`. Los escenarios de consumo y de quiz lanzaban una
excepción en la primera línea de cada iteración, k6 las contaba como
iteraciones completas —2,8 millones en minuto y cuarto— y como ninguna llegó a
hacer una sola petición HTTP, no había errores que contar ni comprobaciones que
suspender. El resumen salía verde.

Se corrigieron las dos cosas, porque son dos fallos distintos:

- El sembrador carga el árbol también al reutilizar, y se niega a escribir un
  escenario sin recursos utilizables.
- `setup()` valida el archivo **entero** antes de empezar: cada escenario
  declara qué campos necesita, y la prueba no arranca si falta alguno. Un
  informe verde que no ejerció lo que dice ejercer no acredita nada, así que
  ahora es imposible producir uno.

### Dónde empieza a doler

Se subió el escalón hasta encontrar el límite. A **1.000 VU**:

- p95 sigue en un dígito de milisegundos (catálogo 4 ms, consumo 11 ms).
- **Ni un solo 5xx** en 120.632 peticiones.
- La tasa de error sube a 0,602 % y el umbral **falla**, que es lo que tiene
  que hacer.

Pero al mirar qué fallaba, no era la plataforma: 725 de los 730 errores eran
`409 Conflict` en `PUT /quiz/attempts/{id}/answers`. Con 200 cuentas sembradas
y 1.000 usuarios virtuales, varios VU compartían estudiante, y mientras uno
guardaba respuestas otro enviaba el intento. El servidor respondía
correctamente —un intento cerrado no admite más respuestas— y la prueba lo
contaba como degradación del servicio.

Era un defecto del arnés, y medir la contención del generador de carga
llamándola degradación del servidor es la peor forma de fallar una prueba de
carga. Ahora `setup()` corta antes de empezar si hay menos cuentas que VU:

```
el escenario tiene 200 cuentas y la prueba pide 400 VU:
siembra al menos 400 (SEED_STUDENTS=400) o baja VUS_OBJETIVO.
```

**Conclusión honesta**: con este escenario no se ha encontrado el límite real
de la plataforma. Lo que se sabe es que a 200 VU sobra capacidad por dos
órdenes de magnitud, y que el siguiente paso para caracterizarla de verdad es
sembrar 2.000 cuentas y subir hasta que el p95 se mueva o aparezca un 5xx.
Hasta entonces, «Etapa 1 pasa» significa exactamente eso y nada más.

## Lo que esta prueba todavía no acredita

El segmento 9 pide más de lo que hay aquí. Queda pendiente, y no se puede
presentar como cubierto:

- **Observabilidad**: hoy hay logs estructurados y correlación por
  `X-Request-Id`; faltan métricas y trazas (OpenTelemetry) que permitan ver
  *dónde* se va el p95 cuando se rompe un umbral. Con los márgenes actuales no
  ha hecho falta; en cuanto se busque el límite real, sí.
- **Fallos inyectados** durante la carga: tirar una réplica de la API a mitad
  de la meseta y demostrar que el p95 se recupera.
- **Backup y restauración** con RPO ≤ 15 min y RTO ≤ 4 h, y la prueba de
  recuperación que los acredita.
- **El escalón de 2.000 concurrentes** del enunciado, que exige sembrar al
  menos esas cuentas y una máquina que no sea la de desarrollo.
