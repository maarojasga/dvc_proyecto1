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

Implementado el primer punto del alcance mínimo (§5.1.1): **registro público de
estudiantes con verificación de correo, sesiones revocables y recuperación de
clave; los profesores solo se crean por administración**. Está completo de
punta a punta: migraciones, dominio, API, frontend y pruebas.

El resto de las pantallas y módulos siguen siendo marcadores navegables que
declaran qué debe implementarse y a qué criterio de evaluación aportan.

## Estructura del repositorio

```
.
├── backend/            Monolito modular en Go (API) + workers independientes
│   ├── cmd/api/            Punto de entrada de la API REST (/api/v1)
│   ├── cmd/worker/         Punto de entrada de los workers asíncronos
│   ├── internal/config/    Lectura de variables de entorno
│   ├── internal/domain/    Entidades y reglas de negocio (sin framework ni cloud)
│   ├── internal/platform/  Adaptadores: HTTP, PostgreSQL, Redis, S3/MinIO, cola
│   └── migrations/         Migraciones SQL de PostgreSQL
├── frontend/           Next.js 16 (App Router), React 19, TypeScript, Tailwind 4
├── docs/               Especificación OpenAPI y notas de arquitectura
└── docker-compose.yml  Postgres, Redis, MinIO, Mailpit, API, workers y frontend
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

### Recorrido manual del flujo de identidad

Con el compose levantado, los enlaces de verificación y recuperación llegan a
Mailpit (http://localhost:8025). La API nunca devuelve esos tokens en la
respuesta HTTP: hacerlo permitiría activar cuentas ajenas.

1. Crear una cuenta en http://localhost:3000/registro
2. Abrir el enlace del correo en Mailpit para activarla
3. Entrar en http://localhost:3000/login
4. Revisar y revocar sesiones en http://localhost:3000/cuenta/sesiones

## Pendientes para las siguientes iteraciones

- Los puntos 2 a 10 del alcance mínimo (§5.1): administración de usuarios,
  autoría y publicación, editor de bloques, carga multimedia, procesamiento
  asíncrono, visores, quizzes, progreso e insignias, y catálogo.
- Workers: hoy el proceso arranca y espera; falta registrar los consumidores
  asynq con reintentos, backoff y dead-letter queue.
- OpenTelemetry: hoy hay logs estructurados y correlación por `X-Request-Id`,
  faltan métricas y trazas.
- Reverse proxy (nginx/traefik) para poder escalar la API a varias instancias:
  hoy publica el puerto 8080 fijo en el host, lo que impide `--scale api=N`.
- Pipeline de CI: build, lint, análisis de seguridad, migraciones y pruebas.
- Pruebas E2E de los nueve flujos críticos y auditoría de accesibilidad.

## Versionado de datos

Se conserva la configuración de [DVC](https://dvc.org) heredada del repositorio
original (`.dvc/`, `.dvcignore`, `data/`) para versionar conjuntos de datos
fuera de git. Si el proyecto no va a manejar datasets, puede eliminarse sin
afectar a la aplicación.
