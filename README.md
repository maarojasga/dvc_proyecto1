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
protección CSRF. Siguen pendientes cursores, ETag y OpenTelemetry.

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
|-- frontend/           Next.js 14 (App Router), React 18, TypeScript y CSS propio
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

## Pendientes para las siguientes iteraciones

El alcance mínimo (sección 5.1) y el opcional (5.2) están cubiertos. Lo que
falta pertenece a las restricciones técnicas (sección 7) y a la demostración de
aceptación (sección 10), ordenado por lo que más pesa para esa demostración:

- **Pruebas E2E** de los nueve flujos críticos y auditoría automática de
  accesibilidad. Es condición de aceptación explícita y hoy no hay ninguna: la
  cobertura llega hasta la API, no hasta el navegador.
- **Pipeline de CI**: build, lint, análisis de seguridad, migraciones y
  pruebas, que la sección 10.1 exige completar antes de la demostración.
- **Prueba de carga de Etapa 1** y el p95 documentado que pide el segmento 9.
- **Reverse proxy** (nginx/traefik) para escalar la API a varias instancias:
  hoy publica el puerto 8080 fijo en el host, lo que impide `--scale api=N`.
  Los workers sí escalan.
- **OpenTelemetry**: hoy hay logs estructurados y correlación por
  `X-Request-Id`; faltan métricas y trazas.
- **Cursores y ETag** en las colecciones, que exige la sección 7.
- **Backup y restauración** con RPO ≤ 15 min y RTO ≤ 4 h, y la prueba de
  recuperación que los acredita.

Queda un borde consciente, no bloqueante: el escáner antimalware integrado no
lleva firmas. Para la demostración conviene levantar el perfil `antivirus` y
apuntar `CLAMAV_ADDR` a clamd.

## Versionado de datos

Queda `data/` con su `.gitignore` heredado del repositorio original, que
reservaba el directorio para conjuntos de datos versionados con
[DVC](https://dvc.org). La configuración de DVC ya no está, así que hoy es solo
un directorio reservado: puede eliminarse sin afectar a la aplicación.
