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
| 4 | Editor de bloques con autosave y Markdown canónico | Completo: bloques con ida y vuelta probada por tipo de nodo, Markdown canónico estable, autoguardado en servidor y borrador local como red de seguridad con recuperación explícita |
| 5 | Carga multimedia | Completo: multipart directa y reanudable 24 h, SHA-256 obligatorio verificado en servidor, MIME real del contenido y escaneo antimalware con ClamAV |
| 6 | Procesamiento asíncrono a HLS | Completo: worker asynq con FFmpeg sin upscaling, original conservado, toma exclusiva del trabajo, reintentos con backoff, dead-letter queue con alerta y entrega autorizada por CDN |
| 7 | Visor PDF y reproducción adaptativa | Completo: reproductor HLS adaptativo que reanuda desde la última posición reportada, visor PDF y entrega autorizada de cada tipo de recurso |
| 8 | Quizzes | Backend completo: snapshot por intento, clave solo en servidor, guardado parcial, envío idempotente, expiración y nota calculada en servidor. Falta la interfaz |
| 9 | Progreso e insignias | Backend completo: heartbeats con permanencia, rechazo auditado de porcentajes del cliente, insignia única y verificación pública sin correo. Faltan la imagen de la insignia y la interfaz |
| 10 | Catálogo, inscripción, retiro y reinscripción | Completo: búsqueda por texto, filtros por categoría y nivel, y la reinscripción reutiliza la inscripción, así que conserva progreso y resultados |

De las restricciones técnicas (sección 7) están resueltas `/api/v1`, OpenAPI
3.1 al día con la implementación, errores uniformes, `Idempotency-Key` y
protección CSRF. Siguen pendientes cursores, ETag y OpenTelemetry.

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
