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

Ver la tabla de cobertura del README de la raíz. En resumen: el alcance mínimo
de la sección 5.1 está cubierto de extremo a extremo —identidad y
administración, autoría versionada, carga verificada, transcodificación a HLS,
consumo, evaluación, progreso e insignias, y catálogo con filtros—. Lo que
queda pertenece a las restricciones técnicas y a la demostración de
aceptación: pruebas E2E, CI, observabilidad con OpenTelemetry, cursores y
ETag.

Las migraciones se aplican solas al arrancar la API, dentro de un
`pg_advisory_lock` para que levantar varias instancias a la vez no las aplique
por duplicado.

## Decisiones de seguridad

| Decisión | Por qué |
|---|---|
| bcrypt con coste 12 para las contraseñas | Derivación lenta y con sal por hash; el coste va dentro, así que subirlo no invalida los hashes existentes. |
| Solo se guarda el hash del token | Ni la sesión ni los enlaces de correo son reconstruibles leyendo la base. |
| Cookie de sesión `httpOnly` + `SameSite=Lax` | Un XSS no puede exfiltrar la sesión y un formulario ajeno no la envía. |
| Doble envío de cookie anti-CSRF | La cookie `mooc_csrf` es legible por el frontend y debe repetirse en `X-CSRF-Token`; un sitio atacante no puede leerla. |
| Respuestas idénticas exista o no la cuenta | Registro, reenvío y recuperación no sirven para enumerar correos. El login compara además contra un hash señuelo para igualar el tiempo de respuesta. |
| La sesión se resuelve contra PostgreSQL en cada petición | No hay caché intermedia que quede obsoleta: revocar surte efecto en la petición siguiente, en cualquier instancia. |
| El token de sesión no viaja en el cuerpo del login | Solo vive en la cookie `httpOnly`; devolverlo dejaría que cualquier script de la página lo leyera. |
| La propiedad se comprueba dentro del `UPDATE` al revocar | No hay ventana entre comprobar y actuar, y una sesión ajena responde igual que una inexistente. |
| Auditoría con disparador que rechaza `UPDATE` y `DELETE` | La bitácora es inmutable en la propia base, no por convención. |
| Límite de tasa en Redis, no en memoria | El cupo es del servicio, así que escalar a N réplicas no multiplica por N el margen del atacante. |
| Tomar un trabajo de transcodificación es un `UPDATE` condicional | Dos entregas del mismo trabajo se resuelven en la base: solo una transcodifica, así que una entrega duplicada no produce salidas repetidas. |
| El arrendamiento del trabajo vence | Si el worker que lo tomó muere, otro lo recoge en vez de dejar el recurso atascado en `processing` para siempre. |
| Toda carga se verifica al confirmarla, sea simple o multipart | El control no depende del camino que elija el cliente: antes bastaba con subir un archivo pequeño, que no pasaba por la multipart, para saltarse checksum, MIME y antimalware. |
| El tipo del contenido se deduce de los bytes, no de lo declarado | `Content-Type` lo escribe quien sube. Lo que decide si un PDF es un PDF son sus primeros bytes. |
| Un escáner que no responde rechaza la carga | Dar por limpio lo que no se pudo escanear convierte apagar el antivirus en una vía de entrada. |
| El objeto rechazado se borra del almacén | Dejarlo conserva un archivo que no superó el control bajo una clave que el recurso ya conoce. |
| El título del curso se escapa al dibujar la insignia | La imagen es un SVG que sirve el almacén y abre el navegador: sin escapar, un título con etiquetas sería un XSS servido por la plataforma. |

## Puesta en marcha

Desde la raíz del repositorio, `docker compose up --build` levanta todo. Para
trabajar solo sobre el backend:

```bash
export DATABASE_URL="postgres://mooc:mooc@localhost:5432/mooc?sslmode=disable"
export REDIS_ADDR=localhost:6379
go run ./cmd/api
```

La API necesita además PostgreSQL, Redis y el almacenamiento de objetos
levantados: comprueba el bucket al arrancar y falla rápido si no responde.

Los enlaces de verificación y recuperación se envían por SMTP (Mailpit en
local) y nunca se devuelven en la respuesta HTTP: hacerlo permitiría activar
cuentas ajenas.

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

El almacenamiento de objetos sí se sustituye por un doble en memoria
(`almacen_falso_test.go`): lo que verifican las pruebas de carga —integridad,
MIME real, antimalware y reanudación— es lógica de la API, y atarlas a MinIO
las dejaría sin ejecutar en la práctica. La firma SigV4, que sí es del
proveedor, se prueba aparte sobre el cliente real en
`internal/platform/storage/firma_test.go`.

Este paquete y el de `postgres` comparten la base de integración y empiezan
vaciando las mismas tablas, así que se serializan entre sí con un cerrojo
consultivo (`internal/platform/postgres/pgtest`). Sin él, `go test ./...` los
ejecuta en paralelo y la suite falla de forma intermitente.

## Variables de entorno

| Variable | Por defecto | Para qué |
|---|---|---|
| `APP_ENV` | `development` | Entorno; fuera de desarrollo la cookie exige `Secure`. |
| `API_PORT` | `8080` | Puerto de escucha. |
| `DATABASE_URL` | `postgres://mooc:mooc@localhost:5432/mooc?sslmode=disable` | Fuente de verdad transaccional. |
| `REDIS_ADDR` | `localhost:6379` | Límites de tasa, idempotencia y cola asynq. |
| `S3_ENDPOINT`, `S3_BUCKET` | `localhost:9000`, `mooc` | Almacenamiento de objetos. |
| `S3_PUBLIC_URL` | — | Base pública (CDN) desde la que se sirven los objetos. Vacío: se firman uno a uno. |
| `S3_ACCESS_KEY`, `S3_SECRET_KEY`, `S3_USE_SSL` | — | Credenciales del almacenamiento. |
| `PUBLIC_BASE_URL` | `http://localhost:3000` | Base de los enlaces de los correos y origen permitido por CORS. |
| `SMTP_HOST`, `SMTP_PORT`, `SMTP_FROM` | `localhost`, `1025`, `no-reply@mooc.local` | Servidor de correo saliente. |
| `CLAMAV_ADDR` | — | Dirección de clamd (`host:puerto`). Vacío: escáner integrado, sin firmas. Configurado y sin respuesta: la carga se rechaza. |
| `SESSION_TTL` | `720h` | Vigencia de la sesión. |
| `COOKIE_SECURE` | `false` | Marca `Secure` en las cookies; actívalo con TLS delante. |
