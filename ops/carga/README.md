# Prueba de carga

Los objetivos del enunciado son **50.000 usuarios registrados** y **2.000
concurrentes**, y la condición de aceptación es que la prueba de carga de
Etapa 1 no presente incumplimientos críticos.

## Qué se considera un incumplimiento

Los umbrales están en `escenarios.js` y son la definición operativa de
«aceptable». Si uno se cruza, k6 sale con código distinto de cero y la
ejecución no acredita nada.

| Flujo | p95 | Por qué ese número |
|---|---|---|
| Catálogo | 500 ms | Lectura paginada por cursor, con índice y cacheable. 500 ms es holgado. |
| Detalle de curso | 500 ms | Igual, más el árbol de módulos. |
| Entrega de recurso | 800 ms | Incluye firmar una URL contra el almacén de objetos, que añade una ida y vuelta. |
| Progreso | 800 ms | Escritura pequeña, pero recalcula el avance del curso. |
| Inicio de sesión | 1.500 ms | **bcrypt con coste 12 tarda ~250 ms a propósito.** Exigir aquí lo mismo que en una lectura sería pedir que el hash fuera débil. |

Además:

- **Disponibilidad**: más del 99,9 % de las comprobaciones tiene que pasar.
- **Peticiones fallidas**: menos del 0,1 %. Se declaran esperados los 2xx y el
  **304**, porque el 304 del catálogo es el acierto del ETag y no un error; los
  4xx quedan fuera, porque un 401 o un 429 durante la prueba significan que el
  sistema está rechazando tráfico legítimo.
- **Errores de negocio**: cero. Una respuesta con forma inesperada no es un
  problema de latencia, es un fallo.
- **Sesiones renovadas**: menos de 5. Si las sesiones se invalidan solas, el
  flujo de consumo estaría midiendo bcrypt con otro nombre.

## Etapas

El enunciado habla de «Etapa 1» sin definirla. La interpretación que se usa
aquí es una escalada, porque medir directamente en el objetivo no dice dónde
está el límite:

| Etapa | Concurrencia | Para qué |
|---|---|---|
| `humo` | 10 | Comprobar que el guion y el juego de datos funcionan. No mide nada. |
| `1` | 200 | 10 % del objetivo. Es la que la condición de aceptación exige. |
| `2` | 1.000 | 50 %. Muestra si la latencia crece de forma lineal o se dispara. |
| `3` | 2.000 | El objetivo completo. |

La subida es gradual (un minuto) a propósito: arrancar 2.000 usuarios de golpe
mide el arranque en frío —pool de conexiones vacío, cachés frías— en lugar del
estado estacionario, que es lo que se quiere conocer.

## Mezcla de tráfico

No es uniforme, porque un MOOC no se usa de forma uniforme:

- **70 % navegar el catálogo** (sin sesión): listar, reenviar el ETag, pasar de
  página con el cursor, abrir un curso.
- **20 % consumir contenido** (con sesión): abrir un curso inscrito, pedir la
  entrega de un recurso y reportar avance con señales de apertura, permanencia
  y cierre.
- **10 % identidad**: iniciar sesión, leer la propia cuenta, cerrar sesión.

Un reparto uniforme daría un p95 que no corresponde a ningún uso real.

El inicio de sesión va en su propio escenario porque bcrypt domina su latencia:
mezclarlo con las lecturas desplazaría el p95 de todo lo demás y ocultaría una
degradación real del catálogo.

## Cómo se ejecuta

### 1. Sembrar el juego de datos

```sh
cd backend
DATABASE_URL=postgres://mooc:mooc@localhost:5432/mooc \
  go run ./cmd/sembrar-carga -estudiantes 50000 -cursos 200 -inscripciones 5
```

El sembrador escribe en la base directamente y con un único hash de contraseña
precalculado. No es un atajo gratuito: bcrypt con coste 12 tarda ~250 ms por
cuenta, así que 50.000 altas por la API serían más de tres horas antes de poder
medir. Por eso mismo el comando **se niega a ejecutarse con
`APP_ENV=production`**: una base real con 50.000 cuentas que comparten una
contraseña conocida es una brecha, no un juego de datos.

Termina con un `ANALYZE`. Sin estadísticas al día, el planificador elige planes
malos sobre las tablas recién llenadas y la primera medición sale peor de lo
que es el sistema.

Para borrarlo: `go run ./cmd/sembrar-carga -limpiar`.

### 2. Ajustar el entorno para medir

```sh
# El generador de carga sale de una sola dirección: con el límite de
# producción (10 por minuto) se bloquearía a sí mismo, y el resultado sería el
# límite de tasa y no el rendimiento del sistema.
LOGIN_RATE_LIMIT=1000000

# Muestreo de trazas bajo: conservar todas durante la prueba mide el coste de
# la instrumentación además del del sistema.
OTEL_TRACES_SAMPLER_ARG=0.05
```

### 3. Ejecutar

```sh
cd ops/carga
CARGA_API=http://localhost:8080 \
CARGA_ESTUDIANTES=50000 \
CARGA_ETAPA=1 \
  k6 run escenarios.js
```

El resultado completo queda en `resultados/etapa-N.json` como evidencia
reproducible.

### 4. Con varias instancias

La prueba de escalamiento es la misma orden contra el proxy, variando el número
de instancias:

```sh
docker compose up -d --scale api=1   # medir
docker compose up -d --scale api=3   # medir de nuevo
```

Si el p95 no mejora al triplicar las instancias, el cuello de botella no está
en la API: normalmente es el pool de conexiones a Postgres o la propia base.

## Variables

| Variable | Por omisión | Qué hace |
|---|---|---|
| `CARGA_API` | `http://localhost:8080` | Base de la API |
| `CARGA_ETAPA` | `1` | `humo`, `1`, `2`, `3` |
| `CARGA_ESTUDIANTES` | `50000` | Cuántas cuentas sembradas hay |
| `CARGA_CORREO_PATRON` | `estudiante%d@carga.local` | Patrón de los correos sembrados |
| `CARGA_CLAVE` | `CargaDePrueba123!` | Contraseña de las cuentas sembradas |
| `CARGA_DURACION` | `5m` | Duración en carga sostenida |
| `CARGA_SUBIDA` | `1m` | Duración de la subida gradual |

## Resultados medidos

Máquina de 4 núcleos, una sola instancia de API, Postgres local, sustituto del
almacén de objetos y **el generador de carga en el mismo host**. Juego de datos
reducido: 2.000 estudiantes, 40 cursos, 6.000 inscripciones.

| Etapa | Concurrencia | req/s | p95 global | Fallos | Umbrales |
|---|---|---|---|---|---|
| humo | 10 | 7 | 3 ms | 0 | todos en verde |
| 1 | 200 | 137 | 8,2 ms | 0 | todos en verde |
| 2 | 1.000 | 358 | 5.449 ms | 0 | **cinco cruzados** |

La etapa 1, que es la que exige la condición de aceptación, pasa con holgura:
el p95 más alto es 8,2 ms frente a umbrales de 500.

### El hallazgo: el inicio de sesión mata de hambre a todo lo demás

La etapa 2 cruza todos los umbrales de latencia **sin un solo fallo**: nada
falla, todo se vuelve lento. Y la latencia está dentro del handler, no en la
cola de aceptación: la propia API registra un p95 de 5.345 ms, igual que el que
mide el cliente.

Repetir la etapa 2 quitando solo el escenario de identidad, con la misma
concurrencia total, lo atribuye sin ambigüedad:

| | catálogo p95 | curso p95 | p95 dentro del handler |
|---|---|---|---|
| Con identidad (100 inicios de sesión concurrentes) | 1.861 ms | 6.485 ms | 5.345 ms |
| Sin identidad | **22 ms** | **42 ms** | **23 ms** |

Cien inicios de sesión concurrentes son unos 25 segundos de CPU por segundo
—bcrypt con coste 12 cuesta ~250 ms de cálculo puro— sobre cuatro núcleos.
El resultado es que un flujo de identidad saturado degrada el catálogo, que no
comparte nada con él salvo el procesador.

Esto no es un defecto que se arregle en el código: bcrypt es caro **a
propósito**, y abaratarlo es debilitar el almacenamiento de contraseñas. Lo que
sí se puede hacer, por orden de efecto:

1. **Escalar horizontalmente.** Es para lo que está el proxy inverso: la API no
   guarda estado, así que `--scale api=N` reparte el coste. Es la mitigación
   principal y ya está en su sitio.
2. **Dar capacidad propia al flujo de identidad.** Con un grupo de instancias
   dedicado a `/auth/*`, un pico de inicios de sesión no puede degradar la
   lectura del catálogo. El proxy ya reparte por ruta, así que es cambiar la
   configuración, no el código.
3. **Reducir la frecuencia de los inicios de sesión.** La sesión dura 30 días
   por defecto; cuanto menos caduque, menos veces se paga el hash.

Reproducirlo:

```sh
CARGA_ETAPA=2 CARGA_ESCENARIOS=navegarCatalogo,consumirContenido k6 run escenarios.js
```

Nota sobre el número de la etapa 2: con el generador de carga en el mismo host,
1.000 usuarios virtuales de k6 también consumen CPU, así que el 5.449 ms es más
pesimista que el del sistema aislado. La **atribución** al coste del hash sí se
sostiene, porque la comparación se hizo con la misma concurrencia y en la misma
máquina.

## Qué NO acredita esta prueba tal como está

- **No se ha ejecutado en el objetivo completo ni en el entorno de la
  demostración.** Lo medido está arriba: máquina de 4 núcleos, una instancia,
  juego de datos reducido y el generador en el mismo host. Sirve para calibrar
  el guion, para atribuir dónde está el cuello y para detectar regresiones; no
  como evidencia de aceptación.
- **La etapa 3 (2.000 concurrentes) no se ha ejecutado.** Con la etapa 2 ya
  saturada en esta máquina, el número que saldría mediría la máquina y no el
  sistema.
- **La evidencia de aceptación exige el sistema desplegado** con Docker
  Compose, el almacén de objetos real y el generador de carga fuera de la
  máquina que ejecuta la API. Con los dos en el mismo host compiten por CPU y
  el número que sale es más pesimista que el real, pero no es el real.
- **No cubre la carga de multimedia.** La subida va directa del navegador al
  almacén de objetos, así que medir esa ruta es medir el almacén, no la
  plataforma. Lo que sí se mide es la firma de URLs, que es la parte que
  atraviesa la API.
- **No inyecta fallos.** El segmento 9 de la demostración pide también fallos,
  copia y restauración: eso está en `docs/operacion.md` y es un ejercicio
  aparte.
