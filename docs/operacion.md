# Operación: copias de seguridad, restauración y recuperación

Los criterios de aceptación piden RPO ≤ 15 minutos y RTO ≤ 4 horas, y que la
recuperación se demuestre. Este documento es el procedimiento, y está escrito
para seguirse durante un incidente: órdenes concretas, en orden, con lo que hay
que comprobar en cada paso.

## Qué hay que respaldar, y qué no

| Qué | Dónde vive | Se respalda | Por qué |
|---|---|---|---|
| Cuentas, cursos, inscripciones, intentos, progreso, insignias, bitácora | PostgreSQL | **Sí**, es la fuente de verdad | Perderlo es perder el producto |
| Originales de multimedia, derivados HLS, PDF, imágenes de insignias | Almacén de objetos | **Sí** | No se pueden reconstruir: el original lo subió una persona |
| Sesiones, caché, límites de tasa, cola | Redis | **No** | Reconstruible. Perderlo cierra sesiones y reencola trabajos, no borra nada |

Redis se queda fuera a propósito. Respaldarlo daría una falsa sensación de
completitud y restaurarlo traería de vuelta límites de tasa y sesiones viejas.
Lo que sí hace falta es que **la pérdida de Redis no pierda trabajos**: los
trabajos de multimedia se reencolan a partir de `media_assets`, que está en
Postgres, comparando su estado con lo que hay en la cola.

## Objetivos y cómo se cumplen

**RPO ≤ 15 min.** Copia completa diaria más archivado continuo de WAL. El WAL
se envía cada 5 minutos como techo (`archive_timeout = 300`), así que lo peor
que se pierde son los últimos 5 minutos, con margen sobre los 15 exigidos. Sin
archivado de WAL —solo la copia diaria— el RPO real sería de 24 horas: la copia
sola no cumple el objetivo.

**RTO ≤ 4 h.** El tiempo lo domina restaurar los objetos, no la base. Por eso
el orden del procedimiento es: primero la base y la API (la plataforma queda
usable, con el material multimedia dando error temporal), y después los
objetos. Un estudiante puede leer, evaluarse y avanzar antes de que termine la
restauración completa.

## Copia de seguridad

### PostgreSQL

```sh
# Copia completa. El formato personalizado permite restaurar tablas sueltas.
docker compose exec -T postgres \
  pg_dump -U mooc -d mooc --format=custom --compress=9 \
  > respaldos/mooc-$(date +%Y%m%dT%H%M%S).dump

# Comprobar que la copia se puede leer. Una copia sin verificar no es una
# copia: es un archivo.
pg_restore --list respaldos/mooc-*.dump | head
```

El archivado continuo de WAL se configura en el servidor de Postgres, no en el
compose de desarrollo:

```
# postgresql.conf
wal_level = replica
archive_mode = on
archive_command = 'test ! -f /archivo/wal/%f && cp %p /archivo/wal/%f'
archive_timeout = 300        # techo de 5 min: acota el RPO
```

### Almacén de objetos

```sh
# Espejo incremental a otro bucket o a otra región.
mc mirror --overwrite --remove local/mooc respaldo/mooc
```

El espejo va con `--remove` para que el respaldo refleje los borrados. Es
deliberado y tiene un riesgo: un borrado por error se propaga. Se compensa con
versionado en el bucket de respaldo, que es lo que permite volver atrás un
borrado sin volver a la copia completa.

### Secretos

No van en las copias. Se gestionan fuera (variables de entorno del despliegue o
un gestor de secretos) y se rotan tras un incidente que pudiera haberlos
expuesto. Una copia de seguridad que contenga las claves convierte cada copia
en un secreto más que proteger.

## Restauración

### 1. Postgres

```sh
# Sobre una base vacía. --clean falla si hay objetos que no existen, así que se
# parte de cero antes que de una base a medias.
createdb -U mooc mooc_restaurada
pg_restore -U mooc -d mooc_restaurada --no-owner respaldos/mooc-20260315T031500.dump
```

Recuperación en un punto del tiempo, si el incidente fue una escritura errónea
y no una pérdida de disco:

```
# recovery.signal + postgresql.conf
restore_command = 'cp /archivo/wal/%f %p'
recovery_target_time = '2026-03-15 04:12:00+00'
```

### 2. Comprobar el esquema antes de abrir

```sh
DATABASE_URL=postgres://mooc:mooc@host:5432/mooc_restaurada go run ./cmd/migrate status
```

Si la copia es de una versión anterior, faltarán migraciones. Aplicarlas:

```sh
DATABASE_URL=... go run ./cmd/migrate up
```

Este paso va **antes** de abrir la API: arrancar contra un esquema viejo
produce errores de columna inexistente que parecen fallos de la aplicación.

### 3. Levantar la API

```sh
DATABASE_URL=postgres://mooc:mooc@host:5432/mooc_restaurada docker compose up -d api worker
curl -fsS http://localhost:8080/api/v1/health
```

A partir de aquí la plataforma funciona salvo la entrega de material
multimedia, que devolverá error hasta que estén los objetos. Es el compromiso
que permite cumplir el RTO.

### 4. Objetos

```sh
mc mirror --overwrite respaldo/mooc local/mooc
```

### 5. Reencolar lo que Redis se llevó

Los trabajos en vuelo se pierden con Redis. La lista de lo pendiente está en
Postgres:

```sql
-- Activos que quedaron a medias: la API los registró y el worker no terminó.
SELECT id, resource_id, status, updated_at
FROM media_assets
WHERE status IN ('uploaded', 'processing')
ORDER BY updated_at;
```

Los que sigan en `processing` con el lease vencido los recupera el propio
worker, porque la toma del trabajo es condicional y con plazo. Los que estén en
`uploaded` hay que reencolarlos desde el panel de operación
(`POST /api/v1/admin/queues/{queue}/dead/{taskId}/retry` para los de la DLQ) o
volviendo a confirmar la carga.

## Prueba de recuperación

No vale con documentarlo. El ejercicio, trimestral:

1. Restaurar la última copia en un entorno aparte.
2. Ejecutar `cmd/migrate status` y comprobar que no falta nada.
3. Levantar la API contra la base restaurada y pasar las pruebas E2E
   (`npm run e2e` en `frontend/`), que recorren los flujos críticos.
4. Anotar el tiempo total y compararlo con el RTO de 4 horas.
5. Anotar la marca de tiempo del último dato recuperado y compararla con el
   RPO de 15 minutos.

El paso 3 es el que convierte el ejercicio en una prueba: una base que
restaura pero sobre la que no se puede aprobar un curso no está recuperada.

## Lo que este procedimiento no cubre todavía

- **No está automatizado.** Las órdenes son manuales; en producción van en una
  tarea programada con alerta si la copia no se completa.
- **No se ha cronometrado en un entorno del tamaño real.** Los objetivos están
  razonados a partir de la configuración, no medidos. La prueba de recuperación
  es lo que los convierte en datos.
- **Cifrado en reposo**: lo aporta el disco cifrado del proveedor y el
  versionado del bucket. No hay cifrado a nivel de aplicación, así que quien
  tenga acceso a la copia tiene acceso a los datos: las copias van a un
  almacenamiento con su propio control de acceso.
