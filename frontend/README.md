# Plataforma MOOC - Frontend

Interfaz de la plataforma web de cursos masivos abiertos en línea. Contiene la
estructura de rutas, el cliente contra la API y el empaquetado en Docker.

**Implementado**: identidad (registro, verificación de correo, inicio y cierre
de sesión, recuperación de clave y gestión de sesiones revocables), catálogo e
inscripción, autoría con previsualización, administración de cuentas y
auditoría, el consumo de contenido (reproductor HLS adaptativo que reanuda
donde se dejó, visor PDF y lectura de recursos de texto), evaluaciones de
selección múltiple (autoría del profesor y presentación del estudiante con
guardado parcial y envío idempotente), progreso verificado por servidor e
insignias digitales (emisión, verificación pública y listado propio).

El README de la raíz describe el monorepo completo y cómo levantar todo el
entorno con Docker Compose.

## Contexto

| | |
|---|---|
| Backend | Go, monolito modular + workers asíncronos (`../backend`) |
| Frontend | Next.js 16 (App Router), React 18, TypeScript |
| Integración | REST JSON contra `/api/v1`, llamado directamente por el navegador |
| Despliegue | Docker y Docker Compose, escalable a múltiples instancias |

## Puesta en marcha

Con Node 20 o superior instalado:

```bash
npm install
npm run dev            # http://localhost:3000
```

Por defecto apunta a una API en `http://localhost:8080`. Para usar otra,
definir `NEXT_PUBLIC_API_URL` antes de arrancar.

Otros comandos:

```bash
npm run build          # compilación de producción (salida standalone)
npm run start          # sirve la compilación
npm run lint           # ESLint
```

### Con Docker

El servicio `frontend` del compose de la raíz construye esta imagen y la
conecta con la API, PostgreSQL, Redis, MinIO y Mailpit:

```bash
cd .. && docker compose up --build
```

## Cómo habla con la API

El navegador llama **directamente** a la API en Go, sin pasar por ningún
proxy ni reescritura de rutas: `lib/api.ts` arma cada petición contra
`NEXT_PUBLIC_API_URL` (por defecto `http://localhost:8080`), con
`credentials: "include"` para que viaje la cookie de sesión httpOnly.

Como son dos orígenes distintos (`localhost:3000` y `localhost:8080`), el
backend tiene CORS habilitado para el origen del frontend
(`PUBLIC_BASE_URL` en su configuración). Las cookies funcionan igual porque
ambos comparten host (`localhost`), que es lo único que importa para
`SameSite`; en un despliegue con dominios distintos de verdad, habría que
revisar esa política.

Las mutaciones (todo lo que no es GET) repiten la cookie legible `mooc_csrf`
en la cabecera `X-CSRF-Token`, que es lo que un sitio atacante no puede
hacer aunque intente falsificar la petición.

## Estructura

```
app/
|-- login/, registro/, verificar-correo/, restablecer-contrasena/   identidad
|-- cuenta/sesiones/                     sesiones activas propias, revocables
|-- mis-cursos/                          inscripciones del estudiante
|-- mis-insignias/                       insignias propias
|-- insignias/[code]/                    verificación pública de una insignia
|-- cursos/[courseId]/                   detalle publicado, progreso, contenido
|   `-- recursos/[resourceId]/           visor por tipo (video, pdf, texto, quiz...)
|-- profesor/                            cursos propios
|   `-- versiones/[versionId]/           estructura, metadatos, publicación
|       |-- previsualizacion/
|       `-- quiz/[resourceId]/           autoría de la evaluación
`-- admin/                               usuarios, roles, auditoría

components/          NavBar, ReproductorHLS, VisorPDF, BlockEditor, QuizPlayer
lib/
|-- api.ts                cliente REST y los tipos que devuelve la API
|-- auth-context.tsx       sesión actual, resuelta preguntándole a la API
`-- useProgressReporting.ts   hook que reporta señales de progreso verificado
```

No hay grupos de rutas ni carpeta `src/`: las páginas viven directo bajo
`app/`, en español para que la URL coincida con lo que ve el usuario.

## Decisiones ya tomadas en la base

- **La clave del quiz nunca viaja al cliente.** `QuizAttempt` expone
  preguntas y opciones sin marcar cuál es la correcta; la calificación la
  hace el servidor contra el snapshot congelado al iniciar el intento.
- **El progreso lo calcula el servidor.** `useProgressReporting` solo avisa
  que el estudiante abrió, sigue ahí o cerró un recurso; nunca envía un
  porcentaje, que la API rechazaría de todas formas.
- **Sin proxy.** El navegador llama directo a la API (ver arriba). Si se
  reintroduce algún día un rewrite de mismo origen, esta sección y la
  variable `NEXT_PUBLIC_API_URL` tendrían que actualizarse juntas.
- **Sin estado local.** La imagen usa la salida `standalone`; cualquier
  instancia es reemplazable.
- **La sesión se resuelve preguntándole a la API.** `auth-context.tsx` llama
  a `/auth/me` al montar: la cookie es opaca para el frontend, y quien decide
  si sigue valiendo es el backend, que puede haberla revocado hace un
  instante.
- **El filtrado por rol en la barra de navegación es presentación.** La
  autorización real la aplica cada endpoint según rol, propiedad e
  inscripción.

## Siguientes pasos

1. Editor de bloques con autosave y Markdown extendido canónico (hoy
   `BlockEditor` cubre un subconjunto).
2. Subtítulos y transcripciones del reproductor HLS.
3. Panel administrativo con métricas y resultados agregados por quiz.
4. Pruebas E2E de los nueve flujos críticos y auditoría automática de
   accesibilidad.
