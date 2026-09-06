# Plataforma MOOC — Frontend

Interfaz de la plataforma web de cursos masivos abiertos en línea. Contiene la
estructura de rutas, el modelo de dominio en TypeScript, la capa de transporte
hacia la API y el empaquetado en Docker.

**Implementado**: identidad (registro, verificación de correo, inicio y cierre
de sesión, recuperación de clave y gestión de sesiones revocables), catálogo e
inscripción, autoría con previsualización, administración de cuentas y
auditoría, y el consumo de contenido: reproductor HLS adaptativo que reanuda
donde se dejó, visor PDF y lectura de recursos de texto.

El README de la raíz describe el monorepo completo y cómo levantar todo el
entorno con Docker Compose.

## Contexto

| | |
|---|---|
| Backend | Go, monolito modular + workers asíncronos (`../backend`) |
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

En desarrollo local sin Docker hay que levantar aparte la API en Go, o apuntar
`API_INTERNAL_URL` a donde esté escuchando.

### Con Docker

El servicio `frontend` del compose de la raíz construye esta imagen y la
conecta con la API, PostgreSQL, Redis, MinIO y Mailpit:

```bash
cd .. && docker compose up --build
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
│   ├── (cuenta)/            sesiones activas del usuario, con revocación
│   └── salud/               sonda usada por el healthcheck de Docker
├── components/
│   ├── auth/                formularios de identidad y gestión de sesiones
│   ├── layout/              cabecera, pie, skip link, navegación lateral
│   └── ui/                  primitivas presentacionales y de formulario
├── lib/
│   ├── api/                 cliente REST, catálogo de endpoints, errores
│   ├── auth/                lectura de la sesión en el servidor
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
  Los campos llevan etiqueta asociada, el error se anuncia con `aria-invalid` y
  `aria-describedby`, y los avisos son regiones vivas.
- **Las mutaciones salen del navegador.** Gracias al rewrite, van al mismo
  origen, así que la API puede fijar la cookie de sesión directamente. El
  cliente repite la cookie `mooc_csrf` en `X-CSRF-Token`, que es lo que un
  sitio atacante no puede hacer.
- **La sesión se resuelve en el servidor.** `src/lib/auth/session.ts` pregunta a
  la API en cada render: la cookie es opaca y quien decide si sigue valiendo es
  el backend, que puede haberla revocado hace un instante.
- **El filtrado por rol de la cabecera es presentación.** La autorización real
  la aplica cada endpoint según rol, propiedad e inscripción.

## Siguientes pasos

1. Catálogo e inscripción contra `endpoints.catalogo` y `endpoints.inscripciones`.
2. Editor de bloques con autosave y Markdown extendido canónico.
3. Carga multipart directa a almacenamiento de objetos con URLs prefirmadas.
4. Reproductor HLS y visor PDF accesible con reporte de progreso por heartbeat.
5. Pruebas E2E de los nueve flujos críticos y auditoría automática de accesibilidad.
