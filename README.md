# Plataforma MOOC — Frontend

Frontend de la plataforma web de cursos masivos abiertos en línea del curso
Cloud. Este repositorio contiene **la base del proyecto**: estructura de rutas,
modelo de dominio en TypeScript, capa de transporte hacia la API y empaquetado
en Docker. **Todavía no hay funcionalidad**: cada pantalla es un marcador
navegable que declara qué debe implementarse y a qué criterio de evaluación
aporta.

## Contexto

| | |
|---|---|
| Backend | Go, monolito modular + workers asíncronos (repositorio aparte) |
| Frontend | Next.js 16 (App Router), React 19, TypeScript, Tailwind CSS 4 |
| Integración | REST JSON sobre HTTPS contra `/api/v1` (OpenAPI 3.1) |
| Despliegue | Docker y Docker Compose, escalable a múltiples instancias |

## Puesta en marcha

```bash
npm install
cp .env.example .env.local
npm run dev            # http://localhost:3000
```

Otros comandos:

```bash
npm run build          # compilación de producción (salida standalone)
npm run start          # sirve la compilación
npm run lint           # ESLint
npm run typecheck      # tsc --noEmit
```

### Con Docker

El backend, PostgreSQL, Redis, MinIO y Mailpit viven en el compose del
repositorio de backend. Este compose se une a la misma red:

```bash
docker network create mooc     # una sola vez, si no existe
docker compose up --build
```

## Estructura

```
src/
├── app/
│   ├── (auth)/              login, registro, verificación de correo, recuperación
│   ├── (publico)/           catálogo y verificación pública de insignias
│   ├── (estudiante)/        mis cursos, visor de recursos, intentos de quiz
│   ├── (profesor)/          autoría: metadatos, estructura, versiones
│   ├── (admin)/             usuarios, auditoría, operación de workers
│   └── salud/               sonda usada por el healthcheck de Docker
├── components/
│   ├── layout/              cabecera, pie, skip link, navegación lateral
│   └── ui/                  primitivas presentacionales
├── lib/
│   ├── api/                 cliente REST, catálogo de endpoints, errores
│   ├── auth/                lectura de sesión (marcador)
│   ├── config/              entorno y navegación
│   └── utils/               formato y utilidades
└── types/                   dominio (curso, quiz, progreso, insignia) y transporte

public/                      activos estáticos servidos desde la raíz del sitio
```

Los grupos de rutas entre paréntesis no aparecen en la URL: sirven para agrupar
por audiencia y darle a cada área su propio layout.

## Decisiones ya tomadas en la base

- **Jerarquía académica.** `src/types/domain.ts` modela Curso → Módulo → Unidad
  → Recurso con `stableId` en cada nivel, para preservar el progreso cuando se
  publica una nueva versión.
- **La clave del quiz nunca viaja al cliente.** `PreguntaQuiz` expone opciones
  sin marcar la correcta; la calificación es responsabilidad del servidor.
- **Convenciones de la API.** El cliente en `src/lib/api/client.ts` deja
  resueltos `Idempotency-Key`, `If-Match`/ETag, credenciales por cookie y el
  formato de error uniforme (`ProblemDetails`).
- **Mismo origen.** `next.config.ts` reescribe `/api/v1/*` hacia la API en Go,
  de modo que no hay CORS y las cookies de sesión permanecen `SameSite`.
- **Sin estado local.** La imagen usa la salida `standalone` y corre como
  usuario sin privilegios; cualquier instancia es reemplazable.
- **Accesibilidad desde el inicio.** `lang="es"`, salto al contenido principal,
  foco visible, respeto a `prefers-reduced-motion` y paleta con contraste AA.

## Siguientes pasos

1. Autenticación real: sesión en `src/lib/auth/session.ts` y protección de las
   áreas por rol, propiedad e inscripción.
2. Catálogo e inscripción contra `endpoints.catalogo` y `endpoints.inscripciones`.
3. Editor de bloques con autosave y Markdown extendido canónico.
4. Carga multipart directa a almacenamiento de objetos con URLs prefirmadas.
5. Reproductor HLS y visor PDF accesible con reporte de progreso por heartbeat.
6. Pruebas E2E de los nueve flujos críticos y auditoría automática de accesibilidad.

## Versionado de datos

Se conserva la configuración de [DVC](https://dvc.org) heredada de
`dvc_proyecto` (`.dvc/`, `.dvcignore`, `data/`) para versionar conjuntos de
datos fuera de git. Si este repositorio no va a manejar datasets, puede
eliminarse sin afectar a la aplicación.
