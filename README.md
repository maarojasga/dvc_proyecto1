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

## Estado

Cobertura del alcance mínimo (sección 5.1 del enunciado):

| # | Punto | Estado |
|---|---|---|
| 1 | Registro, verificación de correo, sesiones revocables y recuperación | Completo, con pruebas de integración |
| 2 | Gestión administrativa de usuarios | Completo: usuarios, roles, estados, sesiones (ver y cerrar), consulta de la bitácora inmutable y protección del último administrador activo |
| 3 | Autoría, jerarquía y versiones | Completo: jerarquía de cuatro niveles con `stable_id`, ordenamiento, previsualización, validación exhaustiva de publicación y versiones publicadas inmutables |
| 4 | Editor de bloques con autosave y Markdown canónico | Completo en lo que el MVP delimita: bloques de encabezado, párrafo, lista, código y aviso, con autoguardado y recuperación del borrador local. La ida y vuelta AST ↔ Markdown cubre ese subconjunto, no el Markdown extendido entero |
| 5 | Carga multimedia | Completo: multipart directa reanudable durante 24 h, checksum SHA-256 extremo a extremo, MIME real deducido de los bytes y escaneo antimalware. Los mismos controles se aplican a la subida simple |
| 6 | Procesamiento asíncrono a HLS | Completo: worker asynq con FFmpeg sin upscaling, original conservado, toma exclusiva del trabajo, reintentos con backoff, dead-letter queue con alerta y entrega autorizada por CDN |
| 7 | Visor PDF y reproducción adaptativa | Completo: reproductor HLS adaptativo que reanuda desde la última posición reportada, visor PDF y entrega autorizada de cada tipo de recurso |
| 8 | Quizzes | Completo: autoría, snapshot congelado al iniciar, guardado parcial, calificación en servidor, envío idempotente, expiración y retroalimentación según la política. La clave correcta nunca sale del servidor |
| 9 | Progreso e insignias | Completo: avance calculado en servidor a partir de heartbeats y permanencia, rechazo auditado de porcentajes enviados por el cliente, aprobación por criterios e insignia única con imagen SVG y URL pública de verificación |
| 10 | Catálogo, inscripción, retiro y reinscripción | Completo: búsqueda por texto y filtros por categoría y nivel, inscripción, retiro y reinscripción conservando progreso |

De las restricciones técnicas (sección 7) están resueltas `/api/v1`, OpenAPI
3.1 al día con la implementación, errores uniformes, `Idempotency-Key` y
protección CSRF. Siguen pendientes cursores, ETag, OpenTelemetry, el cifrado
en reposo y la gestión externa de secretos.

### Alcance opcional (sección 5.2)

| Punto | Estado |
|---|---|
| Conversión de PPTX y ODP a PDF con previsualización | Completo: worker con LibreOffice headless, original conservado, visor de PDF existente |
| Iframes restringidos: lista blanca, sandbox y política de permisos | Completo, con administración de la lista y auditoría de las altas y bajas |
| Editor completo: tablas, fórmulas, tareas e historial de revisiones | Completo: ocho tipos de bloque con 29 pruebas de ida y vuelta, e historial numerado, atribuido y restaurable |
| Borradores de actualización con clasificación de cambios y migración de progreso | Completo: alcance menor/mayor según si cambia lo exigido, y migración de las inscripciones al publicar |
| Panel administrativo con métricas y resultados agregados por quiz | Completo: estado de la plataforma e informe por evaluación con distribución por opción |
| Coautoría básica, exportación de datos personales e internacionalización | Completo: coeditores que no reparten acceso, descarga de datos propios, y la interfaz entera en español e inglés |
| Subtítulos, transcripciones, foros asíncronos y Open Badges 3.0 | Completo: WebVTT con transcripción derivada, foro por curso o lección, y credencial verificable firmada con Ed25519 |

Cuatro notas sobre decisiones que conviene conocer antes de la demostración:

- **La internacionalización cubre la interfaz, no el contenido de los cursos.**
  Las 463 cadenas de las pantallas están en español y en inglés, y el catálogo
  va tipado contra las claves del español: añadir un texto sin traducirlo es un
  error de compilación, no una frase suelta en el idioma equivocado. Lo que no
  se traduce es lo que escribe cada profesor —ya declara su propio idioma en
  `course_versions.language`— ni los identificadores del dominio que la
  interfaz enseña tal cual (el rol de una cuenta, la acción de una entrada de
  auditoría): traducirlos rompería la correspondencia con lo que la API acepta.
  Las fechas y las horas sí siguen al idioma elegido, porque 14/09 y 09/14 no
  son un detalle estético.
- **Los `.ppt` heredados no se aceptan**, y no es un olvido: son contenedores
  OLE y el escáner antimalware los rechaza por admitir macros. El alcance pide
  PPTX y ODP, que son ZIP.
- **La credencial Open Badges se firma solo si hay `BADGE_SIGNING_KEY`.** Sin
  ella la plataforma arranca igual y las insignias siguen siendo verificables
  por su URL pública; lo único que no se ofrece es la credencial portátil. No
  se genera una clave al vuelo como sustituto: cada reinicio produciría otra y
  lo firmado antes dejaría de verificarse.
- **Una insignia revocada no se emite como credencial.** Una credencial firmada
  no se puede desdecir; para el estado está la URL pública de verificación, que
  sigue respondiendo y dice que ya no vale.

```bash
# Clave de firma para las credenciales (Ed25519, en base64):
openssl genpkey -algorithm ed25519 -outform DER | base64 -w0
```

### Verificación de las cargas

Todo binario entra por el mismo sitio, se haya subido de una vez o por partes:
al confirmar, la API recorre el objeto **una sola vez** y con esa lectura
calcula el SHA-256, olfatea el tipo real por los primeros bytes y lo pasa por
el escáner. Si algo no cuadra, el objeto se borra del almacén y la respuesta es
`422` con el motivo en `error.code`:

| `error.code` | Qué ocurrió |
|---|---|
| `checksum_mismatch` | El hash que calculó el navegador no coincide con el objeto almacenado |
| `mime_mismatch` | El contenido real no corresponde al tipo del recurso (un HTML subido como PDF) |
| `malware_detected` | El escaneo reconoció el archivo |
| `upload_missing` | No hay objeto en la clave, o está vacío |
| `scanner_unavailable` (503) | No se pudo escanear. La carga se rechaza: dar por limpio lo que no se escaneó convertiría apagar el antivirus en una vía de entrada |

El escáner tiene dos motores. El **integrado** siempre opera: reconoce el
vector de prueba estándar (EICAR) y bloquea ejecutables nativos, scripts con
shebang y documentos OLE con macros. No lleva firmas, así que no sustituye a un
antivirus. Con `CLAMAV_ADDR` apuntando a un clamd se usan los dos, y ese es el
camino de producción:

```bash
docker compose --profile antivirus up -d clamav   # tarda: descarga las firmas
CLAMAV_ADDR=clamav:3310 docker compose up -d api
```

### Reanudar una subida

La reanudación no la da el endpoint por sí sola. Si el navegador se cierra a
media subida, el identificador de la carga se pierde y las partes que ya
llegaron quedan huérfanas, así que el cliente guarda ese identificador en
`localStorage` y al volver pregunta a
`GET …/multipart/parts` qué partes tiene el almacén antes de enviar nada.

Lo que no sobrevive es el archivo: ningún navegador permite releer un `File`
de una sesión anterior. Por eso la interfaz ofrece «Reanudar subida» y pide
elegir de nuevo el mismo archivo; si el nombre o el tamaño no coinciden, la
carga previa se descarta en vez de producir un objeto mezclado.

Una condición del despliegue que conviene no descubrir en la demostración: el
navegador solo ve la cabecera `ETag` de cada parte si el almacén la publica en
`Access-Control-Expose-Headers`. Sin ella la subida falla al cerrar, porque el
`ETag` es lo que permite ensamblar; el cliente lo detecta y lo dice con ese
mismo mensaje en vez de fallar más adelante con un error opaco.

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
|-- frontend/           Next.js 16 (App Router), React 18, TypeScript y CSS propio
|-- docs/               Especificación OpenAPI, arquitectura y guion de la demostración
|-- postman/            Colección que recorre los nueve segmentos de la demostración
|-- load/               Prueba de carga de Etapa 1 (k6) y sus umbrales
`-- docker-compose.yml  Postgres, Redis, MinIO, Mailpit, API, workers y frontend
```

Cada subproyecto tiene su propio README con el detalle.

## Arquitectura

- **Backend**: Go, monolito modular con el dominio desacoplado del framework
  HTTP y del proveedor cloud; workers independientes sin estado local.
- **Persistencia**: PostgreSQL como fuente de verdad transaccional; Redis para
  el límite de tasa y la cola (asynq).

  El enunciado (sección 4) pide además que Redis lleve las sesiones y la
  caché, y aquí no las lleva. Las sesiones viven en PostgreSQL, en una tabla
  con su hash de token, su vencimiento y su propietario: es lo que hace que
  revocarlas sea transaccional y que sobrevivan a un reinicio de Redis, que
  para «revocación inmediata» pesa más que el ahorro de latencia. Caché no hay
  ninguna, sin más: con los p95 medidos —de 2 a 7 ms— no habría añadido nada
  que no fuera una fuente de datos rancios. Las dos son desviaciones
  conscientes de la regla, no olvidos, pero desviaciones al fin.
- **Almacenamiento de objetos**: S3/MinIO para originales, derivados HLS, PDFs
  e imágenes de insignias; ningún binario vive en la base relacional.
- **Frontend**: Next.js con TypeScript. El navegador llama directamente a la
  API en Go (`NEXT_PUBLIC_API_URL`), sin proxy de por medio; el backend tiene
  CORS habilitado para el origen del frontend y las cookies de sesión viajan
  con `credentials: "include"`.
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
npm ci && npm run lint && npm run typecheck && npm test && npm run build
```

Sin `TEST_DATABASE_URL` y `TEST_REDIS_ADDR`, las pruebas de integración se
omiten en lugar de fallar y solo corren las unitarias.

El frontend trae además 29 pruebas de ida y vuelta entre el AST del editor de
bloques y el Markdown extendido canónico, una por tipo de nodo, que es lo que
recomienda la sección 11 del enunciado. Lo que fijan no es que el Markdown
vuelva letra por letra —eso obligaría a conservar las grafías alternativas que
el formato admite— sino que la forma canónica sea un punto fijo: convertir dos
veces da lo mismo que una, que es lo que importa cuando un documento se abre y
se guarda muchas veces.

El backend trae 259 pruebas: las de dominio corren siempre y las de integración
ejercen la API contra PostgreSQL y Redis reales, porque lo que verifican
—unicidad, consumo atómico de tokens, revocación inmediata, inmutabilidad de
una versión publicada, alcance de cada mutación a su propia versión, toma
exclusiva de un trabajo de transcodificación, límites de tasa e inmutabilidad
de la auditoría— vive en esos adaptadores y un doble de prueba no lo
demostraría.

Dos paquetes de prueba comparten esa base y empiezan vaciando las mismas
tablas, y `go test ./...` ejecuta los paquetes en paralelo. Se serializan con
un cerrojo consultivo de PostgreSQL (`internal/platform/postgres/pgtest`);
sin él la suite fallaba de forma intermitente, con pruebas que pasaban al
ejecutarlas aisladas —justo lo que el enunciado no admite, porque un requisito
solo se acredita con pruebas reproducibles—.

Dos cosas no se ejercitan contra su infraestructura real:

- **La transcodificación**, que exige FFmpeg y almacenamiento de objetos. Sí se
  prueba todo lo que la rodea, que es donde estaban los fallos: la exclusión
  mutua entre entregas, la conservación del original, la lista maestra HLS y la
  alerta al agotar los reintentos.
- **La firma de objetos contra MinIO**, que tiene sus propias pruebas sobre el
  cliente real (`internal/platform/storage/firma_test.go`). Las de carga usan
  un almacén en memoria, porque lo que verifican —integridad, MIME real,
  antimalware y reanudación— es lógica de la API, y exigir MinIO levantado
  dejaría ese control sin pruebas en la práctica.

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

## Demostración de aceptación y prueba de carga

El guion de los nueve segmentos que fija la sección 10.2 del enunciado está en
[`docs/demostracion.md`](docs/demostracion.md), con una nota en cada segmento
sobre qué se puede demostrar hoy y qué no. Cómo grabarlo —orden, superficies,
qué decir y qué no afirmar— está en
[`docs/video-sustentacion.md`](docs/video-sustentacion.md).

La sección 10.1 pide la respuesta de la API como evidencia, y eso en el panel de
red se lee mal. Para enseñarla hay una colección de Postman en
[`postman/`](postman/README.md) que recorre los mismos nueve segmentos y
comprueba lo que cada uno debe acreditar: 55 peticiones y 113 aserciones. **No
sustituye a la prueba de carga**, que sigue siendo cosa de k6 por la razón que
explica ese README.

**No hay que desplegar en un proveedor cloud.** La sección 10.1 pide que la
demostración se ejecute «con datos sintéticos sobre el sistema desplegado
mediante Docker Compose», que es exactamente lo que hay:

```bash
cp .env.example .env
docker compose up -d --build
docker compose --profile carga run --rm seed   # 500 cuentas, un curso, sesiones
docker compose --profile carga run --rm k6     # prueba de carga de Etapa 1
```

**MinIO se descarga de quay.io, no de Docker Hub.** MinIO dejó de publicar su
imagen de forma pública allí: `docker pull minio/minio` responde ahora «pull
access denied … may require 'docker login'». No es un límite de descargas ni un
fallo pasajero, y tumbó una corrida del CI. El compose apunta al registro propio
de MinIO a través de `MINIO_IMAGE`, para que otra mudanza sea un cambio en el
`.env` y no una edición del compose.

Las demás imágenes llevan versión fija —`mailpit:v1.22`, `clamav:1.4.3`,
`k6:1.8.1`, `nginx:1.27-alpine`— porque una etiqueta móvil hace que dos
ejecuciones del mismo commit puedan no ser la misma cosa. La de MinIO es la
única que sigue en `:latest`, y está anotada como deuda.

La API dejó de publicar el puerto 8080 en el host: ahora lo hace un proxy nginx
y las instancias quedan detrás, así que `docker compose up -d --scale api=3`
funciona y el escalamiento a múltiples instancias que pide el criterio de
arquitectura se puede demostrar en lugar de afirmarse. El límite de tasa sigue
contando por IP real, porque la API ya leía `X-Forwarded-For`.

Los umbrales de la prueba de carga —y por qué son esos, dado que el enunciado
fija 2.000 usuarios concurrentes pero ningún p95— están en
[`load/README.md`](load/README.md). k6 sale con código distinto de cero si
alguno se incumple, así que sirven para colgar de ellos un paso de CI.

## Condición de aceptación (sección 10)

El enunciado exige cuatro cosas para aceptar. Estado real, medido:

| Condición | Estado |
|---|---|
| Los nueve flujos críticos superan pruebas **E2E** | **Cubierto.** 34 pruebas en `frontend/e2e/`, una carpeta por segmento, contra la plataforma levantada |
| **Prueba de carga** de Etapa 1 sin incumplimientos críticos | **Ejecutada y superada.** 42.075 peticiones en 4 min, 0 % de error, p95 de 2 ms (catálogo), 5 ms (consumo), 5 ms (quiz) y 277 ms (login). Ver [`load/README.md`](load/README.md) |
| **Auditoría de accesibilidad** sin incumplimientos críticos | **Cubierta.** axe-core sobre WCAG 2.2 A y AA en 13 pantallas, en español y en inglés: cero violaciones |
| **CI** completo antes de la demostración | **Cubierto.** [`.github/workflows/ci.yml`](.github/workflows/ci.yml): build, lint, análisis de seguridad, migraciones, pruebas, E2E, accesibilidad y carga |

```bash
cd frontend && npm run e2e          # los nueve flujos y la accesibilidad
```

Las E2E necesitan la plataforma en marcha y el administrador sembrado
(`E2E_ADMIN_EMAIL` y `E2E_ADMIN_PASSWORD`). Levantan un navegador de verdad:
no simulan la API.

### Por qué Next va en 16 y no en 14

El paso de análisis de seguridad del CI tumbó la rama: `next@14.2.35` arrastra
más de veinte advisories, varios críticos —ejecución remota en el optimizador
de imágenes con AVIF, SSRF en rewrites, envenenamiento de caché en respuestas
de React Server Components, XSS en App Router— y `postcss`, que entra como
dependencia suya, otros cuatro de severidad alta.

No había forma de arreglarlo sin subir de versión mayor: se comprobó que
`15.5.25` todavía deja uno crítico y que solo `16.3.5` los cierra todos. Next
16 acepta React 18, así que el salto no obligó a migrar React. Lo que sí
arrastró fue ESLint 9 con configuración plana, porque `eslint-config-next@16`
lo exige, y la desaparición de `next lint`, que ahora es `eslint .`.

El salto se validó con la suite completa —34 pruebas E2E, la auditoría de
accesibilidad, las 29 unitarias y la compilación—, que es exactamente para lo
que estaba. Ninguna falló.

Dos cosas se corrigieron de paso, porque el episodio las puso a la vista:

- **El Dockerfile del frontend ignoraba el lockfile** (`COPY package.json` y
  `npm install`), así que la imagen desplegada resolvía versiones frescas y
  podía no ser la que validaron las pruebas. Ahora copia `package-lock.json` y
  usa `npm ci`.
- **`next` pasa a `^16.3.5` en vez de un pin exacto.** Un pin exacto es
  justamente cómo `14.2.35` se quedó quieto mientras acumulaba advisories; con
  el lockfile y `npm ci`, la reproducibilidad no depende de congelar el rango.

Para que no vuelva a pasar, `.github/dependabot.yml` vigila npm, Go, las
imágenes base de los contenedores y las propias acciones del pipeline. Los
parches y las versiones menores llegan agrupados en un PR semanal; las mayores
van sueltas y **no se ignoran**, precisamente porque la corrección de estos
advisories era una mayor.

Falta un paso que no se puede dar desde el repositorio: encender
**Dependabot alerts** y **Dependabot security updates** en
Settings → Code security. El archivo configura las actualizaciones de versión;
las de seguridad son las que avisan en cuanto aparece un advisory, sin esperar
al lunes.

### Lo que encontraron estas pruebas

No son decorativas. En la primera pasada destaparon tres defectos que ni las
259 pruebas de backend ni las 29 de frontend podían ver, porque ninguna
cruzaba la frontera entre el navegador y la API:

- **Un profesor no podía añadir ningún recurso.** El editor mandaba el cuerpo
  en PascalCase (`Type`, `Title`) y la API lo declara en snake_case, así que
  toda creación de recurso respondía 400. Es el flujo central de la autoría, y
  estaba roto. Las pruebas de backend no lo veían porque llaman a la API con
  el cuerpo correcto; las de frontend, porque solo cubren la conversión de
  Markdown.
- **La casilla de «opción correcta» de un quiz no tenía etiqueta.** Un profesor
  que redacte con lector de pantalla no podía saber cuál de las casillas marca
  la respuesta buena. axe lo clasifica como crítico.
- **Contraste insuficiente** en el botón de mostrar contraseña: 3,67:1 sobre
  blanco, por debajo del 4,5:1 que exige WCAG 2.2 AA.

Los tres están corregidos, y cada uno tiene ahora una prueba que falla si
vuelve.

## Pendientes para las siguientes iteraciones

El alcance mínimo (sección 5.1), el opcional (5.2) y la condición de
aceptación (sección 10) están cubiertos. Lo que queda pertenece a las
restricciones técnicas de la sección 7 y a la caracterización del sistema:

- **OpenTelemetry**: hoy hay logs estructurados y correlación por
  `X-Request-Id`; faltan métricas y trazas. Con los márgenes actuales de la
  prueba de carga no han hecho falta; para encontrar el límite real, sí.
- **Cursores y ETag** en las colecciones, que exige la sección 7.
- **Backup y restauración** con RPO ≤ 15 min y RTO ≤ 4 h, y la prueba de
  recuperación que los acredita.
- **El escalón de 2.000 concurrentes** del enunciado. La prueba de carga pasa
  Etapa 1 con dos órdenes de magnitud de margen, así que todavía no se sabe
  dónde está el techo: hace falta sembrar esas cuentas y una máquina que no
  sea la de desarrollo.

Queda un borde consciente, no bloqueante: el escáner antimalware integrado no
lleva firmas. Para la demostración conviene levantar el perfil `antivirus` y
apuntar `CLAMAV_ADDR` a clamd.

## Versionado de datos

Queda `data/` con su `.gitignore` heredado del repositorio original, que
reservaba el directorio para conjuntos de datos versionados con
[DVC](https://dvc.org). La configuración de DVC ya no está, así que hoy es solo
un directorio reservado: puede eliminarse sin afectar a la aplicación.
