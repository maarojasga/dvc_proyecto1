# Backend — Plataforma MOOC

Monolito modular en Go con workers asíncronos independientes. El dominio no
conoce HTTP, SQL ni proveedor cloud; los adaptadores viven en
`internal/platform/` y son los únicos que dependen de infraestructura.

## Estructura

```
cmd/api/                 punto de entrada de la API REST (/api/v1)
cmd/worker/              punto de entrada de los workers asíncronos
internal/config/         lectura de variables de entorno
internal/domain/         entidades y reglas de negocio, sin dependencias externas
internal/platform/       adaptadores: HTTP, PostgreSQL, Redis, correo, claves
migrations/              migraciones SQL, incrustadas en el binario
```

## Estado

Implementado el alcance **5.1.1**: registro público de estudiantes con
verificación de correo, sesiones revocables y recuperación de clave. Los demás
paquetes de dominio son marcadores documentados.

Las migraciones se aplican solas al arrancar la API, dentro de un
`pg_advisory_lock` para que levantar varias instancias a la vez no las aplique
por duplicado.

## Decisiones de seguridad

| Decisión | Por qué |
|---|---|
| Argon2id con parámetros dentro del hash | Permite recalibrar el costo sin migrar la base: cada hash se verifica con los suyos. |
| Solo se guarda el hash del token | Ni la sesión ni los enlaces de correo son reconstruibles leyendo la base. |
| Cookie de sesión `httpOnly` + `SameSite=Lax` | Un XSS no puede exfiltrar la sesión y un formulario ajeno no la envía. |
| Doble envío de cookie anti-CSRF | La cookie `mooc_csrf` es legible por el frontend y debe repetirse en `X-CSRF-Token`; un sitio atacante no puede leerla. |
| Respuestas idénticas exista o no la cuenta | Registro, reenvío y recuperación no sirven para enumerar correos. El login compara además contra un hash señuelo para igualar el tiempo de respuesta. |
| PostgreSQL manda sobre la sesión | Redis solo cachea; revocar borra la entrada compartida, así que el efecto es inmediato en todas las instancias. |
| Auditoría con disparador que rechaza `UPDATE` y `DELETE` | La bitácora es inmutable en la propia base, no por convención. |
| Límite de tasa en Redis, no en memoria | El cupo es del servicio, así que escalar a N réplicas no multiplica por N el margen del atacante. |

## Puesta en marcha

Desde la raíz del repositorio, `docker compose up --build` levanta todo. Para
trabajar solo sobre el backend:

```bash
export DATABASE_URL="postgres://mooc:mooc@localhost:5432/mooc?sslmode=disable"
export REDIS_ADDR=localhost:6379
export SMTP_ADDR=            # vacío: los enlaces se registran en el log
go run ./cmd/api
```

`SMTP_ADDR` vacío es un valor deliberado y significa «sin servidor de correo»:
la API escribe los enlaces en el log en lugar de enviarlos. Nunca los devuelve
en la respuesta HTTP, que permitiría activar cuentas ajenas.

## Pruebas

```bash
go vet ./...
go test ./...                                   # solo unitarias
TEST_DATABASE_URL="postgres://mooc:mooc@localhost:5432/mooc?sslmode=disable" \
TEST_REDIS_ADDR=localhost:6379 go test ./...    # + integración
```

Las pruebas de `internal/platform/httpserver` ejercen la API completa contra
PostgreSQL y Redis reales, porque lo que verifican —unicidad, consumo atómico
de tokens, revocación inmediata, límites de tasa, inmutabilidad de la
auditoría— vive precisamente en esos adaptadores. Sin las dos variables se
omiten en lugar de fallar.

## Variables de entorno

| Variable | Por defecto | Para qué |
|---|---|---|
| `APP_ENV` | `development` | Entorno; fuera de desarrollo la cookie exige `Secure`. |
| `API_PORT` | `8080` | Puerto de escucha. |
| `DATABASE_URL` | `postgres://mooc:mooc@localhost:5432/mooc?sslmode=disable` | Fuente de verdad transaccional. |
| `REDIS_ADDR` | `localhost:6379` | Sesiones, caché, límites e idempotencia. |
| `S3_ENDPOINT`, `S3_BUCKET` | `localhost:9000`, `mooc` | Almacenamiento de objetos (aún sin uso). |
| `FRONTEND_URL` | `http://localhost:3000` | Base de los enlaces de los correos. |
| `SMTP_ADDR` | `localhost:1025` | Servidor SMTP; vacío desactiva el envío. |
| `SMTP_FROM`, `SMTP_USER`, `SMTP_PASSWORD` | — | Remitente y credenciales. |
| `COOKIE_SECURE` | según `APP_ENV` | Fuerza o desactiva `Secure` en las cookies. |
