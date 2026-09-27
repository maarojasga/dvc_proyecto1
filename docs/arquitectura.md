# Arquitectura

Notas de diseño de la base. Complementan los README y se actualizan a medida
que se implementa cada módulo. El contrato de la API vive en `openapi.yaml`.

## Frontera entre frontend y backend

El frontend no comparte proceso ni base de datos con el backend. Toda la lectura
y escritura ocurre por REST JSON contra `/api/v1`, y el navegador llama a la API
**directamente**, sin proxy de Next.js de por medio:

```
navegador ──▶ API en Go (NEXT_PUBLIC_API_URL) ──▶ PostgreSQL / Redis
     │                    │
     │                    └──▶ cola asynq ──▶ workers
     │
     └──▶ almacenamiento de objetos / CDN (URLs prefirmadas)
```

Que sea directo tiene consecuencias que no son negociables: es un origen
cruzado, así que la API declara CORS para el origen del frontend y las
peticiones viajan con `credentials: "include"` para que la cookie de sesión
llegue. Un `rewrite` de Next.js evitaría el CORS, pero metería un salto de red
en cada petición y convertiría al frontend en parte del camino crítico.

Los binarios tampoco pasan por Next.js: se suben y descargan con URLs
prefirmadas emitidas por la API, directamente contra el almacenamiento de
objetos (MinIO en local) o el CDN.

## Módulos del backend

El dominio no conoce HTTP, SQL ni el proveedor cloud; los adaptadores viven en
`internal/platform/` y son los únicos que dependen de infraestructura.

| Paquete | Responsabilidad |
|---|---|
| `domain/user` | usuarios, roles y sesiones |
| `domain/course` | jerarquía académica, versiones y reglas de publicación |
| `domain/enrollment` | inscripción, retiro y reinscripción |
| `domain/quiz` | autoría de evaluaciones, intentos y calificación en servidor |
| `domain/progress` | avance validado, transición a `completed` y `approved` |
| `domain/badge` | emisión idempotente, verificación pública y credencial Open Badges 3.0 |
| `domain/cambios` | comparación de un borrador con lo publicado |
| `domain/documento` | reconocimiento de presentaciones PPTX y ODP |
| `domain/iframe` | lista blanca, sandbox y política de permisos |
| `domain/subtitulo` | análisis de WebVTT y derivación de la transcripción |
| `platform/httpserver` | routing, auth, rate limiting, CSRF |
| `platform/postgres` | repositorios sobre la fuente de verdad transaccional |
| `platform/redisclient` | sesiones, caché y límites |
| `platform/queue` | asynq: publicación, consumo, backoff y DLQ |
| `platform/storage` | S3/MinIO: originales, HLS, PDFs e imágenes |
| `platform/antimalware` | escaneo de las cargas: escáner integrado y ClamAV |
| `platform/media` | transcodificación a HLS con FFmpeg |
| `platform/documentos` | conversión de presentaciones a PDF con LibreOffice |

La API y los workers comparten `internal/config` y no guardan estado local, de
modo que cualquier instancia es reemplazable.

## Rutas del frontend

El App Router va plano, con una ruta por pantalla y un solo layout:

| Ruta | Audiencia |
|---|---|
| `/`, `/cursos/[courseId]`, `/insignias/[code]` | cualquiera, sin sesión |
| `/login`, `/registro`, `/verificar-correo`, `/restablecer-contrasena` | anónimos |
| `/mis-cursos`, `/mis-insignias`, `/cuenta/sesiones`, `/cursos/…/recursos/…` | estudiantes |
| `/profesor`, `/profesor/versiones/…` | profesores propietarios |
| `/admin` | administradores |

La autorización efectiva es del servidor. Lo que hace el frontend es no
enseñar lo que no corresponde; nunca sustituye la verificación de rol,
propiedad e inscripción que hace cada endpoint.

## Credenciales portátiles

Una insignia vive en tres formas, y conviene no confundirlas:

- El **registro** en la base, que es la fuente de verdad y la que se revoca.
- La **URL pública de verificación**, que responde siempre y dice si sigue
  vigente. Es lo que se comparte cuando importa el estado actual.
- La **credencial Open Badges 3.0**, un documento firmado que se verifica con
  la clave pública del emisor sin volver a llamar aquí. Es portátil, y por eso
  mismo no puede afirmar un estado: una firma no cambia de opinión. La
  credencial apunta a la URL de verificación en lugar de declararse vigente, y
  una insignia revocada deja de emitirse como credencial.

## Datos que el cliente no debe recibir

- La clave correcta de una pregunta de quiz.
- El correo del estudiante en la verificación pública de una insignia, ni en la
  imagen de la insignia, que se comparte con la misma URL.
- URLs directas a materiales privados sin firma previa.

## Entrada de binarios

Un archivo llega al almacén sin pasar por la API, pero no se acepta como
material hasta que la API lo verifica. La confirmación —la de una subida simple
o el cierre de una multipart— recorre el objeto una sola vez y con esa lectura
calcula el SHA-256, deduce el tipo real de los primeros bytes y lo pasa por el
escáner antimalware. Si algo falla, el objeto se borra.

Que las dos rutas terminen en la misma función es lo que da la garantía: un
control que solo se aplica en el camino largo lo esquiva quien elige el corto.

## Progreso

El avance lo calcula el servidor con heartbeats, permanencia y eventos de
apertura. El cliente reporta señales, no porcentajes: un porcentaje enviado
desde el navegador se rechaza y se audita.

## Accesibilidad

Objetivo WCAG 2.2 AA. La base ya incluye idioma declarado, salto al contenido,
foco visible, soporte de `prefers-reduced-motion` y tokens de color con
contraste verificado. Cada componente nuevo debe ser operable por teclado y
llevar nombre accesible.
