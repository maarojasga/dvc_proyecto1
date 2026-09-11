# Plataforma MOOC

## Integrantes

- Juan David Guzman Casadiego
- Laura Fernanda Martinez Galindo
- Tatiana Luna Perez Suancha
- Maria Alejandra Rojas Garzon

## Descripción

Plataforma web de cursos masivos abiertos en línea (proyecto del curso Cloud).
Administradores, profesores y estudiantes; jerarquía Curso → Módulo → Unidad →
Recurso; multimedia con procesamiento asíncrono a HLS; quizzes calificados en
servidor; progreso validado e insignias digitales verificables.

## Documentación

- `docs/arquitectura.md` — decisiones de arquitectura.
- `docs/openapi.yaml` — contrato de la API (OpenAPI 3.1), al día con el código.
- `docs/operacion.md` — copias de seguridad, restauración y prueba de
  recuperación, con los objetivos de RPO y RTO.

## Estado

Cobertura del alcance mínimo (sección 5.1 del enunciado):

| # | Punto | Estado |
|---|---|---|
| 1 | Registro, verificación de correo, sesiones revocables y recuperación | Completo, con pruebas de integración |
| 2 | Gestión administrativa de usuarios | Completo: usuarios, roles, estados, sesiones (ver y cerrar), consulta de la bitácora inmutable y protección del último administrador activo |
| 3 | Autoría, jerarquía y versiones | Completo: jerarquía de cuatro niveles con `stable_id`, ordenamiento, previsualización, validación exhaustiva de publicación y versiones publicadas inmutables |
| 4 | Editor de bloques con autosave y Markdown canónico | Completo: bloques con ida y vuelta probada por tipo de nodo, Markdown canónico estable, autoguardado en servidor y borrador local como red de seguridad con recuperación explícita |
| 5 | Carga multimedia | Completo: multipart directa y reanudable 24 h, SHA-256 obligatorio verificado en servidor, MIME real del contenido y escaneo antimalware con ClamAV |
| 6 | Procesamiento asíncrono a HLS | Completo: worker asynq con FFmpeg sin upscaling, original conservado, toma exclusiva del trabajo, reintentos con backoff, dead-letter queue con alerta y entrega autorizada por CDN |
| 7 | Visor PDF y reproducción adaptativa | Completo: reproductor HLS adaptativo que reanuda desde la última posición reportada, visor PDF y entrega autorizada de cada tipo de recurso |
| 8 | Quizzes | Completo: autoría con clave, presentación del intento, guardado parcial, envío idempotente, expiración y nota calculada en servidor |
| 9 | Progreso e insignias | Completo: latidos con permanencia, rechazo auditado de porcentajes del cliente, insignia única con imagen y verificación pública sin datos personales |
| 10 | Catálogo, inscripción, retiro y reinscripción | Completo: búsqueda por texto, filtros por categoría y nivel, y la reinscripción reutiliza la inscripción, así que conserva progreso y resultados |

De las restricciones técnicas (sección 7) están resueltas `/api/v1`, OpenAPI
3.1 al día con la implementación, errores uniformes, `Idempotency-Key`,
protección CSRF, paginación por cursor, ETag con peticiones condicionales y
OpenTelemetry con trazas y métricas.

### Evaluación y progreso

La clave correcta no sale del servidor nunca. Viaja una sola vez, del profesor
al servidor, al definir la evaluación; a partir de ahí el intento que ve el
estudiante lleva enunciados y opciones y nada más, y la nota llega calculada.
El cliente no tiene con qué recomponerla, que es la única forma de que no se
pueda.

El avance también lo calcula el servidor, a partir de los latidos y de los
eventos de apertura y cierre. El cliente reporta hechos —abrí, sigo aquí,
cerré— y nunca porcentajes: un porcentaje enviado se rechaza y se audita.

Dos detalles que se pagan y no son evidentes:

- **El cierre del recurso se manda con `keepalive`.** Es la señal que acredita
  el material que no se mide por tiempo (texto, imagen, enlace), y una petición
  normal lanzada mientras la página se descarga la cancela el navegador. Sin
  eso, un recurso leído nunca cuenta como visto y el curso no se puede
  terminar. Se manda también en `pagehide` y al ocultarse la pestaña, porque la
  limpieza de un efecto no corre cuando alguien cierra la pestaña.
- **La imagen de la insignia es un SVG generado al emitirla** y guardado en el
  almacén de objetos, no en Postgres. Lleva el curso, la fecha y el código, y
  no lleva nada del estudiante: la imagen se comparte, y lo público no puede
  exponer datos personales. El título del curso va escapado, porque lo escribe
  un profesor.

### Autoría del contenido de texto

El editor de bloques guarda Markdown canónico: la serialización es estable, así
que dos ediciones que no cambian nada no producen documentos distintos. Hay
pruebas de ida y vuelta por tipo de nodo, que es lo que el enunciado recomienda
dejar bloqueante, y fijan dos propiedades distintas: que bloques → Markdown →
bloques devuelve los mismos bloques, y que Markdown → bloques → Markdown es
estable a partir de la primera pasada.

El autoguardado va al servidor, con el borrador en `localStorage` solo como red
de seguridad por si el guardado falla. La diferencia se dice en la interfaz
—«guardado en el servidor» y «borrador local» son garantías distintas— y el
borrador de una sesión anterior **se ofrece, no se impone**: cargarlo por
encima del contenido del servidor pierde ediciones sin avisar, y basta editar
en un equipo y abrir en otro donde quedó un borrador viejo para que el
autoguardado lo escriba encima.

### Verificación de las cargas

El material no pasa por la API: el navegador lo sube directo al almacén con
URLs prefirmadas, en partes de 5 MB, y la carga queda abierta 24 horas, así que
una subida interrumpida se reanuda pidiendo qué partes llegaron y subiendo solo
las que faltan.

Al cerrar la carga se comprueban cuatro cosas, en este orden: que el objeto
existe y no está vacío, que su SHA-256 coincide con el que declaró el cliente,
que su MIME **real** —el de los bytes, no el declarado— no es de los
prohibidos, y que pasa el antimalware. Si algo falla, el objeto se borra: dejar
material que ya se decidió rechazar solo sirve para que alguien lo encuentre
después.

El checksum es obligatorio y lo calcula el navegador antes de subir. Uno
opcional no verifica integridad: la documenta cuando ya salió bien.

Dos detalles que se pagan al desplegar:

- **El almacén tiene que exponer la cabecera `ETag` por CORS**
  (`Access-Control-Expose-Headers: ETag`). El cierre de la carga necesita el
  ETag de cada parte, y sin la cabecera expuesta el navegador lee `null`. Es un
  fallo incómodo porque el PUT responde 200: parece que fue bien.
- **`ANTIMALWARE_ADDR` apunta al demonio ClamAV.** Si se deja vacía, la API usa
  un escáner de desarrollo que solo reconoce EICAR y firmas de ejecutable, y
  con `APP_ENV=production` el arranque falla en lugar de aceptar cargas sin
  escanear de verdad. Un escáner que aprueba todo es peor que ninguno, porque
  nadie lo revisa.

### Prueba de carga

Los objetivos son 50.000 usuarios registrados y 2.000 concurrentes. La prueba
está en `ops/carga/` con k6, por etapas, y sus umbrales por flujo son la
definición operativa de «sin incumplimientos críticos» —incluido uno que hay
que justificar: **el inicio de sesión tiene un umbral de 1.500 ms y no 500,
porque bcrypt con coste 12 tarda ~250 ms a propósito.** Pedirle a ese flujo lo
mismo que a una lectura sería pedir que el hash fuera débil.

Sembrar el juego de datos va por la base directamente y con un único hash
precalculado, no por la API: por ese mismo coste de bcrypt, 50.000 altas serían
más de tres horas antes de poder medir. El comando se niega a arrancar con
`APP_ENV=production`, porque 50.000 cuentas con una contraseña conocida en una
base real es una brecha.

Lo medido hasta ahora, en una máquina de 4 núcleos con una sola instancia de
API y el generador de carga en el mismo host:

| Etapa | Concurrencia | req/s | p95 global | Fallos | Umbrales |
|---|---|---|---|---|---|
| 1 (10 % del objetivo) | 200 | 137 | 8,2 ms | 0 | en verde |
| 2 (50 %) | 1.000 | 358 | 5.449 ms | 0 | cinco cruzados |

La etapa 1, que es la que exige la condición de aceptación, pasa con holgura.
La etapa 2 cruza todos los umbrales de latencia **sin un solo fallo**: nada
falla, todo se vuelve lento.

Y la causa está atribuida. Repetir la etapa 2 quitando solo el escenario de
inicio de sesión, con la misma concurrencia total, baja el p95 del catálogo de
1.861 ms a **22 ms**. Cien inicios de sesión concurrentes son unos 25 segundos
de CPU por segundo sobre cuatro núcleos, porque bcrypt con coste 12 cuesta
~250 ms de cálculo puro: **un pico de identidad degrada la lectura del
catálogo, que no comparte nada con él salvo el procesador.**

No es un defecto que se arregle en el código —abaratar bcrypt es debilitar el
almacenamiento de contraseñas—. Se mitiga escalando horizontalmente, que es
para lo que está el proxy inverso, y dándole capacidad propia al flujo de
identidad.

**Nada de esto es la evidencia de aceptación**: exige el sistema desplegado con
Compose, el almacén real y el generador de carga fuera de la máquina que ejecuta
la API. Los números, la atribución y lo que la prueba no cubre están en
`ops/carga/README.md`.

### Pruebas y CI

| Qué | Cuántas | Dónde |
|---|---|---|
| Unitarias y de integración del backend | Toda la suite, con Postgres y Redis reales | `go test ./... -p 1` |
| Unitarias del frontend | 45 | `npm test` |
| Flujos críticos y accesibilidad, extremo a extremo | 19 | `npm run e2e` |

`-p 1` en el backend no es un detalle: las pruebas de integración truncan las
mismas tablas, así que en paralelo se destruyen entre ellas.

Las E2E cubren los nueve segmentos de la demostración del enunciado y necesitan
la pila levantada. La auditoría de accesibilidad falla con lo grave (`critical`
y `serious`) e informa de lo menor: poner el umbral en «cualquier aviso» hace
que nadie mire el informe, que es peor que no tenerlo.

Una auditoría automática no sustituye una revisión manual. Detecta contraste,
etiquetas, roles y orden de encabezados; no detecta si el texto de un enlace
tiene sentido. Lo que acredita es que no hay incumplimientos automáticamente
detectables.

**El límite de tasa de identidad es configurable** (`LOGIN_RATE_LIMIT`) porque
una suite E2E o una prueba de carga salen de una sola dirección y con el valor
de producción —10 por minuto— se bloquearían a sí mismas. El valor por defecto
es el de producción, y hay una prueba que comprueba que el límite funciona.

### Paginación por cursor y ETag

El catálogo pagina por cursor, no por desplazamiento. Con `OFFSET`, si se
publica un curso mientras alguien pasa de página, las filas se desplazan y el
lector se salta una o ve otra dos veces; y el coste crece con el
desplazamiento, porque la base cuenta y descarta todo lo anterior. El cursor
apunta a la última fila entregada, así que es estable frente a inserciones y su
coste no depende de lo lejos que se haya llegado. Hay una prueba que publica un
curso a mitad del recorrido y comprueba que no se repite ni se salta ninguno.

El orden lleva el `id` como desempate. Sin él, dos versiones publicadas en el
mismo instante dejarían el orden incompleto, y la paginación volvería a
repetir o saltarse filas.

El cursor es opaco y un cursor inválido responde 400, no la primera página:
devolver el principio en silencio haría creer al cliente que está avanzando.

El catálogo y el árbol de un curso publicado responden con `ETag`, y un
`If-None-Match` que coincide recibe 304 sin cuerpo. El ETag se calcula del JSON
que se iba a enviar, que es más trabajo que derivarlo de un `updated_at` pero
no se puede desincronizar. **`/resources/{id}/content` no lleva ETag** a
propósito: su respuesta incluye una URL firmada con caducidad, y un 304 dejaría
al cliente con una URL vencida creyendo que está al día.

### Escalar a varias instancias

`docker compose up --scale api=3` funciona porque la API **no publica puerto**:
lo hace un nginx delante, que reparte entre las instancias. Con la API
publicando 8080 directamente —como estaba— la segunda instancia chocaba con el
puerto de la primera y el escalamiento fallaba.

El proxy resuelve `api` en cada petición y no al arrancar (de ahí la variable
en `proxy_pass`), porque el DNS del compose devuelve todas las instancias del
servicio: sin eso, nginx se quedaría con la primera dirección y escalar no
cambiaría nada.

Esto es además lo que hace demostrable que la API no guarda estado local: las
peticiones de una misma sesión caen en instancias distintas y todo sigue
funcionando, porque la sesión vive en Redis y la verdad en Postgres.

**`TRUSTED_PROXIES` hay que declararlo detrás del proxy.** De la dirección del
cliente dependen el límite de tasa y la bitácora, y con un proxy delante todas
las peticiones llegan desde él. Sin declararlo, el límite se aplicaría a la
suma de todo el tráfico y un solo abusador lo agotaría para todos.

Declarar de más es peor que de menos: `X-Forwarded-For` la escribe quien
quiera, y creerla sin condiciones —como se hacía— deja esquivar el límite de
tasa entero rotando el valor, y llena la bitácora de direcciones elegidas por
el atacante. Ahora la cabecera solo se lee cuando la petición llega de una red
declarada, y se toma el último salto de la cadena que no sea de confianza.

### Observabilidad

Trazas y métricas por OpenTelemetry, exportadas por OTLP/HTTP al colector que
indique `OTEL_EXPORTER_OTLP_ENDPOINT`. Vacía desactiva la exportación: en local
no hay colector, y fallar el arranque por eso sería peor que no tener trazas.

Lo que más cuesta conseguir y más se nota en la demostración es que **la traza
cruza la cola**. La API inyecta el contexto de traza en el propio trabajo y el
worker lo extrae al ejecutarlo, así que subir un vídeo y verlo transcodificado
son el mismo hilo y no dos trazas sueltas. El transporte son las cabeceras
estándar de W3C Trace Context, así que cualquier colector lo entiende sin
configuración especial.

El muestreo es `ParentBased`: si la API decidió conservar una petición, el
worker que continúa ese trabajo mantiene la decisión. Si cada pieza decidiera
por su cuenta quedarían trazas a medias, que es lo peor de los dos mundos —se
paga el coste y no se puede seguir el hilo—. Para la prueba de carga conviene
bajar `OTEL_TRACES_SAMPLER_ARG`.

El nombre de cada tramo sale del patrón de ruta (`GET /api/v1/catalog/{courseId}`)
y no de la URL concreta: con la URL, cada identificador crearía un nombre
distinto y las métricas serían inservibles por cardinalidad. El chequeo de
salud no se instrumenta, porque lo llama el orquestador cada pocos segundos y
no dice nada de nadie.

### El host del almacén: interno frente al del navegador

La API habla con MinIO por el nombre de la red del compose (`minio:9000`), pero
las URLs prefirmadas —la de subida y la de descarga— las abre el **navegador**,
que no resuelve ese nombre. Firmar contra el host interno produce exactamente
esto:

```
PUT http://minio:9000/mooc/resources/.../original?X-Amz-Signature=...
net::ERR_NAME_NOT_RESOLVED
```

No se arregla reescribiendo la URL después de firmarla: la firma SigV4 cubre la
cabecera `Host`, así que cambiar el host invalida la firma. Por eso hay dos
clientes: uno interno, que transfiere bytes, y otro que solo firma, apuntando a
`S3_PUBLIC_ENDPOINT` (por defecto `localhost:${MINIO_API_PORT}`). El cliente de
firma lleva la región fijada a propósito, porque si no minio-go la descubriría
con una petición real contra un host que desde el contenedor no resuelve.

Un fallo de subida arrastra al de publicación: sin objeto, el recurso se queda
en `processing_status != ready`, y publicar responde `422` con el motivo
`hay recursos visibles que aún no terminan de procesarse`. Si aparece ese 422,
la causa suele estar una pantalla más atrás, en la subida.

### Entrega del contenido multimedia

`S3_PUBLIC_URL` fija la base pública desde la que se sirven los objetos, que en
producción es el CDN. Con ella configurada, `GET /resources/{id}/content`
devuelve la URL del manifiesto en el CDN; sin ella, devuelve una URL firmada de
15 minutos.

Dos condiciones del despliegue que conviene no descubrir en la demostración:

- **Los segmentos los pide el reproductor con rutas relativas al manifiesto**,
  así que su autorización la resuelve el CDN. Sin CDN delante, el prefijo
  `hls/` del bucket debe ser legible por el reproductor: firmar solo el
  manifiesto no alcanza para los segmentos.
- **El origen del frontend debe estar permitido por CORS en el almacenamiento
  o el CDN.** Safari reproduce HLS de forma nativa y no lo necesita, pero el
  resto de navegadores usan hls.js, que lee el manifiesto y los segmentos por
  fetch y queda bloqueado sin `Access-Control-Allow-Origin`.

### Reanudar la reproducción

`PUT /resources/{id}/position` registra el segundo donde reanudar. Es una
comodidad reportada por el cliente y **no acredita avance**: el progreso lo
calcula el servidor a partir de heartbeats y eventos de apertura, así que la
posición vive en su propia columna y no toca el estado ni el tiempo acreditado.
Se guarda contra el `stable_id` del recurso, de modo que sobrevive a la
publicación de una versión nueva.

## Estructura del repositorio

```
.
|-- backend/            Monolito modular en Go (API) + workers independientes
|   |-- cmd/api/            Punto de entrada de la API REST (/api/v1)
|   |-- cmd/worker/         Punto de entrada de los workers asíncronos
|   |-- internal/config/    Lectura de variables de entorno
|   |-- internal/domain/    Entidades y reglas de negocio (sin framework ni cloud)
|   |-- internal/platform/  Adaptadores: HTTP, PostgreSQL, Redis, S3/MinIO, cola
|   `-- migrations/         Migraciones SQL de PostgreSQL
|-- frontend/           Next.js 16 (App Router), React 19, TypeScript, Tailwind 4
|-- docs/               Especificación OpenAPI y notas de arquitectura
`-- docker-compose.yml  Postgres, Redis, MinIO, Mailpit, API, workers y frontend
```

Cada subproyecto tiene su propio README con el detalle.

## Arquitectura

- **Backend**: Go, monolito modular con el dominio desacoplado del framework
  HTTP y del proveedor cloud; workers independientes sin estado local.
- **Persistencia**: PostgreSQL como fuente de verdad transaccional; Redis para
  sesiones, caché, rate limiting y la cola (asynq).
- **Almacenamiento de objetos**: S3/MinIO para originales, derivados HLS, PDFs
  e imágenes de insignias; ningún binario vive en la base relacional.
- **Frontend**: Next.js con TypeScript. El navegador llama a `/api/v1` del
  mismo origen y Next reescribe hacia la API en Go, así no hay CORS y las
  cookies de sesión permanecen `SameSite`.
- **Despliegue**: Docker y Docker Compose, con API y workers preparados para
  escalar a múltiples instancias.

## Cómo levantar el entorno local

```bash
cp .env.example .env
docker compose up --build
```

| Servicio | URL |
|---|---|
| Frontend | http://localhost:3000 |
| API | http://localhost:8080/api/v1/health |
| MinIO (S3) | http://localhost:9100 — consola http://localhost:9101 |
| Mailpit | http://localhost:8025 |

Los puertos de MinIO son configurables con `MINIO_API_PORT` y
`MINIO_CONSOLE_PORT` en el `.env`, porque 9000/9001 suelen estar ocupados por
otros proyectos.

### Escalar instancias

```bash
docker compose up -d --scale worker=3
```

## Verificación

```bash
cd backend
go vet ./... && go build ./...
TEST_DATABASE_URL="postgres://mooc:mooc@localhost:5432/mooc?sslmode=disable" \
TEST_REDIS_ADDR=localhost:6379 go test ./...

cd ../frontend
npm ci && npm run lint && npm run typecheck && npm run build
```

Sin `TEST_DATABASE_URL` y `TEST_REDIS_ADDR`, las pruebas de integración se
omiten en lugar de fallar y solo corren las unitarias.

El backend trae 108 pruebas: las de dominio corren siempre y las de integración
ejercen la API contra PostgreSQL y Redis reales, porque lo que verifican
—unicidad, consumo atómico de tokens, revocación inmediata, inmutabilidad de
una versión publicada, alcance de cada mutación a su propia versión, toma
exclusiva de un trabajo de transcodificación, límites de tasa e inmutabilidad
de la auditoría— vive en esos adaptadores y un doble de prueba no lo
demostraría.

La transcodificación en sí no se ejercita en las pruebas: exige FFmpeg y
almacenamiento de objetos. Lo que sí se prueba es todo lo que la rodea, que es
donde estaban los fallos: la exclusión mutua entre entregas, la conservación
del original, la lista maestra HLS y la alerta al agotar los reintentos.

### Recorrido manual del flujo de identidad

Con el compose levantado, los enlaces de verificación y recuperación llegan a
Mailpit (http://localhost:8025). La API nunca devuelve esos tokens en la
respuesta HTTP: hacerlo permitiría activar cuentas ajenas.

1. Crear una cuenta en http://localhost:3000/registro
2. Abrir el enlace del correo en Mailpit para activarla
3. Entrar en http://localhost:3000/login
4. Revisar y revocar sesiones en http://localhost:3000/cuenta/sesiones

Para la autoría y la administración hace falta un profesor o un administrador,
que por diseño no se crean por registro público: un administrador invita
profesores desde http://localhost:3000/admin, y el primer administrador se
siembra con `ADMIN_EMAIL` y `ADMIN_PASSWORD` en el `.env`.

## Pendientes para las siguientes iteraciones

Ordenados por lo que más falta para la demostración de aceptación:

- **Quizzes, progreso e insignias** (puntos 8 y 9). El dominio ya está escrito
  y probado; falta el repositorio, la API y la interfaz que lo usen.
- **Editor de bloques** (punto 4): hoy la autoría usa campos de texto Markdown
  sin autosave ni AST canónico.
- **Carga multimedia** (punto 5): multipart reanudable, verificación de
  checksum, MIME real y escaneo antimalware. Los ayudantes multipart ya están
  en `internal/platform/storage`, pero ningún endpoint los usa todavía.
- **Cursores y ETag** en las colecciones, que exige la sección 7.
- **OpenTelemetry**: hoy hay logs estructurados y correlación por
  `X-Request-Id`; faltan métricas y trazas.
- **Reverse proxy** (nginx/traefik) para escalar la API a varias instancias:
  hoy publica el puerto 8080 fijo en el host, lo que impide `--scale api=N`.
  Los workers sí escalan.
- **Pipeline de CI**: build, lint, análisis de seguridad, migraciones y pruebas.
- **Pruebas E2E** de los nueve flujos críticos y auditoría de accesibilidad.

## Versionado de datos

Queda `data/` con su `.gitignore` heredado del repositorio original, que
reservaba el directorio para conjuntos de datos versionados con
[DVC](https://dvc.org). La configuración de DVC ya no está, así que hoy es solo
un directorio reservado: puede eliminarse sin afectar a la aplicación.
