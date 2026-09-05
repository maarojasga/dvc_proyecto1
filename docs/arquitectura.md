# Arquitectura del frontend

Notas de diseño de la base. Complementan el README y se actualizan a medida que
se implementa cada módulo.

## Relación con el backend

El frontend no comparte proceso ni base de datos con el backend. Toda la lectura
y escritura ocurre por REST JSON contra `/api/v1`:

```
navegador ──▶ Next.js (rewrite /api/v1/*) ──▶ API en Go ──▶ PostgreSQL / Redis
                                                   │
                                                   └──▶ cola asynq ──▶ workers
```

Los binarios no pasan por Next.js: se suben y descargan con URLs prefirmadas
emitidas por la API, directamente contra el almacenamiento de objetos (MinIO en
local) o el CDN.

## Grupos de rutas

Cada grupo tiene una audiencia y un layout propio:

| Grupo | Audiencia | Requiere sesión |
|---|---|---|
| `(publico)` | cualquiera | no |
| `(auth)` | anónimos | no |
| `(estudiante)` | estudiantes inscritos | sí |
| `(profesor)` | profesores propietarios | sí |
| `(admin)` | administradores | sí |

La autorización efectiva es del servidor. El agrupamiento por rol organiza la
interfaz; nunca sustituye la verificación de rol, propiedad e inscripción que
hace cada endpoint.

## Datos que el cliente no debe recibir

- La clave correcta de una pregunta de quiz.
- El correo del estudiante en la verificación pública de una insignia.
- URLs directas a materiales privados sin firma previa.

## Progreso

El avance lo calcula el servidor con heartbeats, permanencia y eventos de
apertura. El cliente reporta señales, no porcentajes: un porcentaje enviado
desde el navegador se rechaza y se audita.

## Accesibilidad

Objetivo WCAG 2.2 AA. La base ya incluye idioma declarado, salto al contenido,
foco visible, soporte de `prefers-reduced-motion` y tokens de color con
contraste verificado. Cada componente nuevo debe ser operable por teclado y
llevar nombre accesible.
