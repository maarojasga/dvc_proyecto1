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
| 4 | Editor de bloques con autosave y Markdown canónico | No: hoy son campos de texto Markdown sin autosave |
| 5 | Carga multimedia | Parcial: PUT prefirmado de 24 h. Falta multipart reanudable, checksum, MIME real y antimalware |
| 6 | Procesamiento asíncrono a HLS | Completo: worker asynq con FFmpeg sin upscaling, original conservado, toma exclusiva del trabajo, reintentos con backoff, dead-letter queue con alerta y entrega autorizada por CDN |
| 7 | Visor PDF y reproducción adaptativa | No |
| 8 | Quizzes | No: el dominio existe con pruebas, pero sin repositorio, API ni interfaz |
| 9 | Progreso e insignias | No: igual que quizzes |
| 10 | Catálogo, inscripción, retiro y reinscripción | Casi completo. Falta ampliar filtros más allá de la búsqueda por texto |

De las restricciones técnicas (sección 7) están resueltas `/api/v1`, OpenAPI
3.1 al día con la implementación, errores uniformes, `Idempotency-Key` y
protección CSRF. Siguen pendientes cursores, ETag y OpenTelemetry.

### Entrega del contenido multimedia

`S3_PUBLIC_URL` fija la base pública desde la que se sirven los objetos, que en
producción es el CDN. Con ella configurada, `GET /resources/{id}/playback`
devuelve la URL del manifiesto en el CDN; sin ella, devuelve una URL firmada de
15 minutos.

El reproductor pide los segmentos con rutas relativas al manifiesto, así que su
autorización la resuelve el CDN. Sin CDN delante, el prefijo `hls/` del bucket
debe ser legible por el reproductor: firmar solo el manifiesto no alcanza para
los segmentos.

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

El backend trae 93 pruebas: las de dominio corren siempre y las de integración
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
- **Consumo de contenido** (punto 7): visor PDF y reproductor HLS con
  reanudación desde la última posición reportada. La URL de reproducción ya la
  entrega la API; falta el reproductor.
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
