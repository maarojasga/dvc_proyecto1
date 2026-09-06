# Migraciones

Migraciones SQL de PostgreSQL para usuarios, sesiones, auditoría, cursos,
módulos, unidades, recursos, multimedia, inscripciones, progreso, quizzes e
insignias.

Convención de nombres: `NNNN_descripcion.up.sql` / `NNNN_descripcion.down.sql`.

Los archivos `.sql` de este directorio se embeben en el binario de la API
mediante `//go:embed` (ver `embed.go`) y se aplican automáticamente al
arrancar (`internal/platform/postgres.Migrate`), registrando cada migración
aplicada en la tabla `schema_migrations`. No se requiere una herramienta de
migración externa, pero el nombre de archivo es compatible con
[golang-migrate](https://github.com/golang-migrate/migrate) si en el futuro
se prefiere ejecutarlas fuera del binario.
