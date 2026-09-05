# Plataforma MOOC

## Integrantes

- Juan David Guzman Casadiego
- Laura Fernanda Martinez Galindo
- Tatiana Luna Perez Suancha
- Maria Alejandra Rojas Garzon

## Descripción

Plataforma web de cursos masivos abiertos en línea (proyecto del curso Cloud).
Administradores, profesores y estudiantes; jerarquía Curso -> Módulo -> Unidad ->
Recurso; multimedia con procesamiento asíncrono a HLS; quizzes calificados en
servidor; progreso validado e insignias digitales verificables.

## Estructura del repositorio

```
.
|-- backend/            Monolito modular en Go (API) + workers independientes
|   |-- cmd/api/        Punto de entrada de la API REST (/api/v1)
|   |-- cmd/worker/     Punto de entrada de los workers asíncronos
|   |-- internal/domain/    Entidades y reglas de negocio (sin dependencias de framework)
|   |-- internal/platform/  Adaptadores: HTTP, PostgreSQL, Redis, S3/MinIO, cola (asynq)
|   `-- migrations/     Migraciones SQL de PostgreSQL
|-- frontend/           Next.js + TypeScript
|-- docs/               Especificación OpenAPI y documentación técnica
`-- docker-compose.yml  Orquestación local: Postgres, Redis, MinIO, Mailpit, API, workers, frontend
```

## Arquitectura

- **Backend**: Go, monolito modular con dominio desacoplado del framework HTTP
  y del proveedor cloud; workers independientes sin estado local.
- **Persistencia**: PostgreSQL como fuente de verdad transaccional; Redis para
  sesiones, caché, rate limiting y la cola (asynq).
- **Almacenamiento de objetos**: S3/MinIO para originales, derivados HLS, PDFs
  e imágenes de insignias (nunca en la base relacional).
- **Frontend**: Next.js + TypeScript, consume la API vía REST/JSON sobre HTTPS.
- **Despliegue**: Docker y Docker Compose, con la API y los workers preparados
  para escalar a múltiples instancias.

## Cómo levantar el entorno local

```bash
cp .env.example .env
docker compose up --build
```

- API: http://localhost:8080/api/v1/health
- Frontend: http://localhost:3000
- MinIO (S3): http://localhost:9100, consola: http://localhost:9101
- Mailpit: http://localhost:8025

Los puertos de MinIO son configurables con `MINIO_API_PORT` y
`MINIO_CONSOLE_PORT` en el `.env`, porque 9000/9001 suelen estar ocupados por
otros proyectos.

### Escalar instancias

```bash
docker compose up -d --scale worker=3
```

## Estado actual

Este commit contiene el scaffolding inicial (estructura de carpetas, stubs de
API/worker, Next.js base y Docker Compose), verificado end-to-end: los siete
servicios levantan, la API responde en `/api/v1/health`, el frontend sirve la
página y los workers escalan a múltiples instancias.

Pendiente para las siguientes iteraciones:

- Esquema de base de datos y migraciones SQL.
- Implementación funcional de cada dominio y los endpoints de la API.
- Reverse proxy (nginx/traefik) para poder escalar la API a varias instancias:
  hoy publica el puerto 8080 fijo en el host, lo que impide `--scale api=N`.
- `package-lock.json` del frontend commiteado, para poder usar `npm ci` en CI.
