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

Este repositorio contiene **la base del proyecto**, sin funcionalidad todavía:
la estructura de carpetas, los puntos de entrada de API y workers, y el
frontend con sus rutas como marcadores navegables. Cada pieza declara qué debe
implementarse y a qué criterio de evaluación aporta.

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

## Verificación de la base

```bash
cd backend  && go vet ./... && go build ./...
cd frontend && npm ci && npm run lint && npm run typecheck && npm run build
```

## Pendientes para las siguientes iteraciones

- Esquema de base de datos y migraciones SQL.
- Implementación funcional de cada dominio y los endpoints de la API.
- Reverse proxy (nginx/traefik) para poder escalar la API a varias instancias:
  hoy publica el puerto 8080 fijo en el host, lo que impide `--scale api=N`.
- Pipeline de CI: build, lint, análisis de seguridad, migraciones y pruebas.
- Pruebas E2E de los nueve flujos críticos y auditoría de accesibilidad.

## Versionado de datos

Se conserva la configuración de [DVC](https://dvc.org) heredada del repositorio
original (`.dvc/`, `.dvcignore`, `data/`) para versionar conjuntos de datos
fuera de git. Si el proyecto no va a manejar datasets, puede eliminarse sin
afectar a la aplicación.
