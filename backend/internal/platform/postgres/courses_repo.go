package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/DES-SOLUCIONES-CLOUD/proyecto-1/backend/internal/domain/course"
)

type CourseRepo struct{ pool *pgxpool.Pool }

func NewCourseRepo(pool *pgxpool.Pool) *CourseRepo { return &CourseRepo{pool: pool} }

func (r *CourseRepo) CreateCourse(ctx context.Context, c *course.Course) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO courses (id, teacher_id, slug, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5)`, c.ID, c.TeacherID, c.Slug, c.CreatedAt, c.UpdatedAt)
	return err
}

func (r *CourseRepo) GetCourse(ctx context.Context, id uuid.UUID) (*course.Course, error) {
	var c course.Course
	err := r.pool.QueryRow(ctx, `
		SELECT id, teacher_id, slug, current_published_version_id, created_at, updated_at
		FROM courses WHERE id=$1`, id).
		Scan(&c.ID, &c.TeacherID, &c.Slug, &c.CurrentPublishedVersionID, &c.CreatedAt, &c.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return &c, err
}

func (r *CourseRepo) GetCourseBySlug(ctx context.Context, slug string) (*course.Course, error) {
	var c course.Course
	err := r.pool.QueryRow(ctx, `
		SELECT id, teacher_id, slug, current_published_version_id, created_at, updated_at
		FROM courses WHERE slug=$1`, slug).
		Scan(&c.ID, &c.TeacherID, &c.Slug, &c.CurrentPublishedVersionID, &c.CreatedAt, &c.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return &c, err
}

func (r *CourseRepo) ListByTeacher(ctx context.Context, teacherID uuid.UUID) ([]*course.Course, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, teacher_id, slug, current_published_version_id, created_at, updated_at
		FROM courses WHERE teacher_id=$1 ORDER BY created_at DESC`, teacherID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*course.Course
	for rows.Next() {
		var c course.Course
		if err := rows.Scan(&c.ID, &c.TeacherID, &c.Slug, &c.CurrentPublishedVersionID, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, &c)
	}
	return out, rows.Err()
}

func (r *CourseRepo) SetCurrentPublishedVersion(ctx context.Context, courseID, versionID uuid.UUID) error {
	_, err := r.pool.Exec(ctx, `UPDATE courses SET current_published_version_id=$2, updated_at=now() WHERE id=$1`, courseID, versionID)
	return err
}

// --- Versiones ---

const versionColumns = `id, course_id, version_number, status, title, summary, description_md,
	coalesce(category,''), coalesce(level,''), language, approval_min_score,
	approval_required_resources_pct, published_at, created_at, updated_at`

func scanVersion(row pgx.Row) (*course.Version, error) {
	var v course.Version
	err := row.Scan(&v.ID, &v.CourseID, &v.VersionNumber, &v.Status, &v.Title, &v.Summary, &v.DescriptionMD,
		&v.Category, &v.Level, &v.Language, &v.ApprovalMinScore, &v.ApprovalRequiredResourcesPct,
		&v.PublishedAt, &v.CreatedAt, &v.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return &v, err
}

func (r *CourseRepo) CreateVersion(ctx context.Context, v *course.Version) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO course_versions (id, course_id, version_number, status, title, summary, description_md,
			category, level, language, approval_min_score, approval_required_resources_pct, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)`,
		v.ID, v.CourseID, v.VersionNumber, v.Status, v.Title, v.Summary, v.DescriptionMD,
		nullIfEmpty(v.Category), nullIfEmpty(v.Level), v.Language, v.ApprovalMinScore,
		v.ApprovalRequiredResourcesPct, v.CreatedAt, v.UpdatedAt)
	return err
}

func (r *CourseRepo) GetVersion(ctx context.Context, id uuid.UUID) (*course.Version, error) {
	row := r.pool.QueryRow(ctx, `SELECT `+versionColumns+` FROM course_versions WHERE id=$1`, id)
	return scanVersion(row)
}

func (r *CourseRepo) LatestVersionNumber(ctx context.Context, courseID uuid.UUID) (int, error) {
	var n int
	err := r.pool.QueryRow(ctx, `SELECT coalesce(max(version_number),0) FROM course_versions WHERE course_id=$1`, courseID).Scan(&n)
	return n, err
}

func (r *CourseRepo) UpdateVersionMetadata(ctx context.Context, v *course.Version) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE course_versions SET title=$2, summary=$3, description_md=$4, category=$5, level=$6,
			language=$7, approval_min_score=$8, approval_required_resources_pct=$9, updated_at=$10
		WHERE id=$1 AND status='draft'`,
		v.ID, v.Title, v.Summary, v.DescriptionMD, nullIfEmpty(v.Category), nullIfEmpty(v.Level),
		v.Language, v.ApprovalMinScore, v.ApprovalRequiredResourcesPct, v.UpdatedAt)
	return err
}

func (r *CourseRepo) TransitionVersionStatus(ctx context.Context, id uuid.UUID, status course.VersionStatus, publishedAt *time.Time) error {
	_, err := r.pool.Exec(ctx, `UPDATE course_versions SET status=$2, published_at=coalesce($3, published_at), updated_at=now() WHERE id=$1`,
		id, status, publishedAt)
	return err
}

// ListCatalog devuelve versiones publicadas con filtros de búsqueda simples.
type CatalogFilter struct {
	Search   string
	Category string
	Level    string
	Limit    int
	Offset   int
}

func (r *CourseRepo) ListCatalog(ctx context.Context, f CatalogFilter) ([]*course.Version, error) {
	limit := f.Limit
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	rows, err := r.pool.Query(ctx, `
		SELECT `+versionColumns+` FROM course_versions
		WHERE status='published'
		  AND ($1 = '' OR title ILIKE '%'||$1||'%' OR summary ILIKE '%'||$1||'%')
		  AND ($2 = '' OR category = $2)
		  AND ($3 = '' OR level = $3)
		ORDER BY published_at DESC
		LIMIT $4 OFFSET $5`, f.Search, f.Category, f.Level, limit, f.Offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*course.Version
	for rows.Next() {
		v, err := scanVersion(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// --- Módulos, unidades, recursos ---

// affectedOne traduce "ninguna fila afectada" a no encontrado: el objetivo no
// existe o no pertenece a la versión autorizada, y desde fuera ambas cosas se
// responden igual para no confirmar identificadores ajenos.
func affectedOne(tag pgconn.CommandTag, err error) error {
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *CourseRepo) CreateModule(ctx context.Context, m *course.Module) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO modules (id, course_version_id, stable_id, title, position)
		VALUES ($1,$2,$3,$4,$5)`, m.ID, m.CourseVersionID, m.StableID, m.Title, m.Position)
	return err
}

// Las operaciones sobre módulos, unidades y recursos llevan el identificador
// de la versión en la propia consulta.
//
// La ruta autoriza al profesor sobre una versión, pero el módulo o el recurso
// llegan como identificadores sueltos: sin acotar por versión, un profesor
// podría pasar el identificador de un módulo de otro curso y modificarlo.
// Comprobarlo en el SQL, y no antes, evita además la ventana entre comprobar
// y actuar.

func (r *CourseRepo) UpdateModule(ctx context.Context, versionID uuid.UUID, m *course.Module) error {
	tag, err := r.pool.Exec(ctx, `
		UPDATE modules SET title=$3, position=$4, updated_at=now()
		WHERE id=$1 AND course_version_id=$2`, m.ID, versionID, m.Title, m.Position)
	return affectedOne(tag, err)
}

func (r *CourseRepo) DeleteModule(ctx context.Context, versionID, id uuid.UUID) error {
	tag, err := r.pool.Exec(ctx,
		`DELETE FROM modules WHERE id=$1 AND course_version_id=$2`, id, versionID)
	return affectedOne(tag, err)
}

// CreateUnit inserta solo si el módulo pertenece a la versión autorizada.
func (r *CourseRepo) CreateUnit(ctx context.Context, versionID uuid.UUID, u *course.Unit) error {
	tag, err := r.pool.Exec(ctx, `
		INSERT INTO units (id, module_id, stable_id, title, position)
		SELECT $1, $2, $3, $4, $5
		  FROM modules WHERE id=$2 AND course_version_id=$6`,
		u.ID, u.ModuleID, u.StableID, u.Title, u.Position, versionID)
	return affectedOne(tag, err)
}

func (r *CourseRepo) UpdateUnit(ctx context.Context, versionID uuid.UUID, u *course.Unit) error {
	tag, err := r.pool.Exec(ctx, `
		UPDATE units SET title=$3, position=$4, updated_at=now()
		WHERE id=$1 AND module_id IN (SELECT id FROM modules WHERE course_version_id=$2)`,
		u.ID, versionID, u.Title, u.Position)
	return affectedOne(tag, err)
}

func (r *CourseRepo) DeleteUnit(ctx context.Context, versionID, id uuid.UUID) error {
	tag, err := r.pool.Exec(ctx, `
		DELETE FROM units
		WHERE id=$1 AND module_id IN (SELECT id FROM modules WHERE course_version_id=$2)`,
		id, versionID)
	return affectedOne(tag, err)
}

// unidadesDeLaVersion acota una consulta de recursos a la versión autorizada.
const unidadesDeLaVersion = `
	SELECT u.id FROM units u
	  JOIN modules m ON m.id = u.module_id
	 WHERE m.course_version_id = $2`

// CreateResource inserta solo si la unidad pertenece a la versión autorizada.
func (r *CourseRepo) CreateResource(ctx context.Context, versionID uuid.UUID, res *course.Resource) error {
	tag, err := r.pool.Exec(ctx, `
		INSERT INTO resources (id, unit_id, stable_id, type, title, position, visible, required,
			downloadable, processing_status, text_content_md, external_url, object_key)
		SELECT $1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13
		  FROM units u JOIN modules m ON m.id = u.module_id
		 WHERE u.id = $2 AND m.course_version_id = $14`,
		res.ID, res.UnitID, res.StableID, res.Type, res.Title, res.Position, res.Visible, res.Required,
		res.Downloadable, res.ProcessingStatus, nullIfEmpty(res.TextContentMD), nullIfEmpty(res.ExternalURL),
		nullIfEmpty(res.ObjectKey), versionID)
	return affectedOne(tag, err)
}

func (r *CourseRepo) UpdateResource(ctx context.Context, versionID uuid.UUID, res *course.Resource) error {
	tag, err := r.pool.Exec(ctx, `
		UPDATE resources SET title=$3, position=$4, visible=$5, required=$6, downloadable=$7,
			processing_status=$8, text_content_md=$9, external_url=$10, object_key=$11, updated_at=now()
		WHERE id=$1 AND unit_id IN (`+unidadesDeLaVersion+`)`,
		res.ID, versionID, res.Title, res.Position, res.Visible, res.Required, res.Downloadable,
		res.ProcessingStatus, nullIfEmpty(res.TextContentMD), nullIfEmpty(res.ExternalURL), nullIfEmpty(res.ObjectKey))
	return affectedOne(tag, err)
}

func (r *CourseRepo) DeleteResource(ctx context.Context, versionID, id uuid.UUID) error {
	tag, err := r.pool.Exec(ctx,
		`DELETE FROM resources WHERE id=$1 AND unit_id IN (`+unidadesDeLaVersion+`)`, id, versionID)
	return affectedOne(tag, err)
}

func (r *CourseRepo) GetResource(ctx context.Context, versionID, id uuid.UUID) (*course.Resource, error) {
	var res course.Resource
	err := r.pool.QueryRow(ctx, `
		SELECT id, unit_id, stable_id, type, title, position, visible, required, downloadable,
			processing_status, coalesce(text_content_md,''), coalesce(external_url,''), coalesce(object_key,'')
		FROM resources WHERE id=$1 AND unit_id IN (`+unidadesDeLaVersion+`)`, id, versionID).
		Scan(&res.ID, &res.UnitID, &res.StableID, &res.Type, &res.Title, &res.Position, &res.Visible,
			&res.Required, &res.Downloadable, &res.ProcessingStatus, &res.TextContentMD, &res.ExternalURL, &res.ObjectKey)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return &res, err
}

// SetResourceProcessingStatusInternal cambia el estado de procesamiento sin
// acotar por versión.
//
// Es para los workers: su autoridad viene del trabajo encolado por la API, no
// de una sesión de usuario, así que no hay una versión autorizada contra la
// que acotar. No debe usarse desde un handler HTTP.
// RecursoPublicadoConMedia describe un recurso de la versión vigente de su
// curso junto con el estado de su activo multimedia.
type RecursoPublicadoConMedia struct {
	CourseID     uuid.UUID
	TeacherID    uuid.UUID
	StableID     uuid.UUID
	Type         string
	Title        string
	Visible      bool
	Downloadable bool
	TextContent  string
	ExternalURL  string
	ObjectKey    string
	AssetStatus  string
	HLSMasterKey string
}

// GetRecursoPublicadoConMedia resuelve un recurso solo si pertenece a la
// versión vigente del curso.
//
// La condición sobre current_published_version_id va en la consulta: así un
// recurso de un borrador o de una versión retirada no se puede reproducir
// aunque se conozca su identificador.
func (r *CourseRepo) GetRecursoPublicadoConMedia(ctx context.Context, resourceID uuid.UUID) (*RecursoPublicadoConMedia, error) {
	var out RecursoPublicadoConMedia
	err := r.pool.QueryRow(ctx, `
		SELECT c.id, c.teacher_id, res.stable_id, res.type, res.title, res.visible,
		       res.downloadable, coalesce(res.text_content_md, ''),
		       coalesce(res.external_url, ''), coalesce(res.object_key, ''),
		       coalesce(ma.status, ''), coalesce(ma.hls_master_key, '')
		  FROM resources res
		  JOIN units u   ON u.id = res.unit_id
		  JOIN modules m ON m.id = u.module_id
		  JOIN course_versions v ON v.id = m.course_version_id
		  JOIN courses c ON c.id = v.course_id AND c.current_published_version_id = v.id
		  LEFT JOIN media_assets ma ON ma.resource_id = res.id
		 WHERE res.id = $1`, resourceID).
		Scan(&out.CourseID, &out.TeacherID, &out.StableID, &out.Type, &out.Title, &out.Visible,
			&out.Downloadable, &out.TextContent, &out.ExternalURL, &out.ObjectKey,
			&out.AssetStatus, &out.HLSMasterKey)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func (r *CourseRepo) SetResourceProcessingStatusInternal(ctx context.Context, id uuid.UUID, status course.ProcessingStatus) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE resources SET processing_status=$2, updated_at=now() WHERE id=$1`, id, status)
	return err
}

func (r *CourseRepo) SetResourceProcessingStatus(ctx context.Context, versionID, id uuid.UUID, status course.ProcessingStatus) error {
	tag, err := r.pool.Exec(ctx, `
		UPDATE resources SET processing_status=$3, updated_at=now()
		WHERE id=$1 AND unit_id IN (`+unidadesDeLaVersion+`)`, id, versionID, status)
	return affectedOne(tag, err)
}

// LoadTree carga módulos, unidades y recursos completos de una versión, en
// orden de posición, necesario para previsualización y validación de
// publicación.
func (r *CourseRepo) LoadTree(ctx context.Context, versionID uuid.UUID) ([]course.Module, error) {
	modRows, err := r.pool.Query(ctx, `
		SELECT id, course_version_id, stable_id, title, position FROM modules
		WHERE course_version_id=$1 ORDER BY position`, versionID)
	if err != nil {
		return nil, err
	}
	var modules []course.Module
	for modRows.Next() {
		var m course.Module
		if err := modRows.Scan(&m.ID, &m.CourseVersionID, &m.StableID, &m.Title, &m.Position); err != nil {
			modRows.Close()
			return nil, err
		}
		modules = append(modules, m)
	}
	modRows.Close()
	if err := modRows.Err(); err != nil {
		return nil, err
	}

	for mi := range modules {
		unitRows, err := r.pool.Query(ctx, `
			SELECT id, module_id, stable_id, title, position FROM units
			WHERE module_id=$1 ORDER BY position`, modules[mi].ID)
		if err != nil {
			return nil, err
		}
		var units []course.Unit
		for unitRows.Next() {
			var u course.Unit
			if err := unitRows.Scan(&u.ID, &u.ModuleID, &u.StableID, &u.Title, &u.Position); err != nil {
				unitRows.Close()
				return nil, err
			}
			units = append(units, u)
		}
		unitRows.Close()
		if err := unitRows.Err(); err != nil {
			return nil, err
		}

		for ui := range units {
			resRows, err := r.pool.Query(ctx, `
				SELECT id, unit_id, stable_id, type, title, position, visible, required, downloadable,
					processing_status, coalesce(text_content_md,''), coalesce(external_url,''), coalesce(object_key,'')
				FROM resources WHERE unit_id=$1 ORDER BY position`, units[ui].ID)
			if err != nil {
				return nil, err
			}
			var resources []course.Resource
			for resRows.Next() {
				var res course.Resource
				if err := resRows.Scan(&res.ID, &res.UnitID, &res.StableID, &res.Type, &res.Title, &res.Position,
					&res.Visible, &res.Required, &res.Downloadable, &res.ProcessingStatus,
					&res.TextContentMD, &res.ExternalURL, &res.ObjectKey); err != nil {
					resRows.Close()
					return nil, err
				}
				resources = append(resources, res)
			}
			resRows.Close()
			if err := resRows.Err(); err != nil {
				return nil, err
			}
			units[ui].Resources = resources
		}
		modules[mi].Units = units
	}

	return modules, nil
}

// PublishVersionAtomic despublica la versión previamente publicada (si la
// hay), publica la nueva versión y actualiza el puntero del curso, todo en
// una sola transacción: nunca puede quedar el curso sin versión vigente a
// medio camino.
func (r *CourseRepo) PublishVersionAtomic(ctx context.Context, courseID uuid.UUID, previousPublishedVersionID *uuid.UUID, newVersionID uuid.UUID, now time.Time) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if previousPublishedVersionID != nil {
		if _, err := tx.Exec(ctx, `UPDATE course_versions SET status='unpublished', updated_at=$2 WHERE id=$1`,
			*previousPublishedVersionID, now); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(ctx, `UPDATE course_versions SET status='published', published_at=$2, updated_at=$2 WHERE id=$1`,
		newVersionID, now); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE courses SET current_published_version_id=$2, updated_at=$3 WHERE id=$1`,
		courseID, newVersionID, now); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// UnpublishVersionAtomic retira una versión publicada y limpia el puntero
// del curso, permitiendo su edición (5.1: "la edición de un curso publicado
// exige despublicarlo temporalmente").
func (r *CourseRepo) UnpublishVersionAtomic(ctx context.Context, courseID, versionID uuid.UUID, now time.Time) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `UPDATE course_versions SET status='unpublished', updated_at=$2 WHERE id=$1 AND status='published'`,
		versionID, now); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE courses SET current_published_version_id=NULL, updated_at=$2 WHERE id=$1 AND current_published_version_id=$3`,
		courseID, now, versionID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}
