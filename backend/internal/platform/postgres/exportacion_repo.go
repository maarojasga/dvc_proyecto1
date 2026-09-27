package postgres

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ExportacionRepo reúne todo lo que la plataforma guarda sobre una persona
// (alcance opcional 5.2: exportación de datos personales).
//
// La consulta va por tablas y no por un volcado genérico a propósito: una
// exportación tiene que poder explicarse. Cada bloque de esta estructura
// corresponde a algo que la persona reconoce —sus cursos, sus evaluaciones,
// sus insignias— y no al nombre interno de una tabla.
type ExportacionRepo struct{ pool *pgxpool.Pool }

func NewExportacionRepo(pool *pgxpool.Pool) *ExportacionRepo { return &ExportacionRepo{pool: pool} }

// DatosPersonales es el paquete que se entrega.
type DatosPersonales struct {
	GeneradoEn time.Time        `json:"generado_en"`
	Cuenta     CuentaExportada  `json:"cuenta"`
	Sesiones   []SesionResumida `json:"sesiones"`
	// Inscripciones lleva dentro el progreso y los intentos de cada curso: ese
	// es el orden en que la persona entiende sus propios datos.
	Inscripciones []InscripcionExportada `json:"inscripciones"`
	Insignias     []InsigniaExportada    `json:"insignias"`
	// Auditoria son las acciones registradas sobre la propia cuenta.
	Auditoria []AccionAuditada `json:"auditoria"`
}

type CuentaExportada struct {
	ID               uuid.UUID  `json:"id"`
	Email            string     `json:"email"`
	NombreCompleto   string     `json:"nombre_completo"`
	Rol              string     `json:"rol"`
	Estado           string     `json:"estado"`
	CorreoVerifadoEn *time.Time `json:"correo_verificado_en,omitempty"`
	CreadaEn         time.Time  `json:"creada_en"`
}

// SesionResumida describe una sesión sin el hash del token: exportar el
// material con que se autentica una sesión sería entregar la llave junto con
// el inventario.
type SesionResumida struct {
	CreadaEn   time.Time  `json:"creada_en"`
	VenceEn    time.Time  `json:"vence_en"`
	RevocadaEn *time.Time `json:"revocada_en,omitempty"`
	IP         string     `json:"ip,omitempty"`
	UserAgent  string     `json:"user_agent,omitempty"`
}

type InscripcionExportada struct {
	CursoID      uuid.UUID           `json:"curso_id"`
	CursoTitulo  string              `json:"curso_titulo"`
	Estado       string              `json:"estado"`
	InscritoEn   time.Time           `json:"inscrito_en"`
	CompletadoEn *time.Time          `json:"completado_en,omitempty"`
	AprobadoEn   *time.Time          `json:"aprobado_en,omitempty"`
	Progreso     []ProgresoExportado `json:"progreso"`
	Intentos     []IntentoExportado  `json:"intentos_de_evaluacion"`
}

type ProgresoExportado struct {
	RecursoStableID   uuid.UUID  `json:"recurso_stable_id"`
	Estado            string     `json:"estado"`
	TiempoSegundos    int        `json:"tiempo_dedicado_segundos"`
	PrimeraAperturaEn *time.Time `json:"primera_apertura_en,omitempty"`
	CompletadoEn      *time.Time `json:"completado_en,omitempty"`
}

// IntentoExportado no lleva el snapshot del quiz: son las preguntas y la clave
// correcta del profesor, no datos de la persona, y exportarlos repartiría el
// examen.
type IntentoExportado struct {
	Numero     int        `json:"numero"`
	Estado     string     `json:"estado"`
	Nota       *float64   `json:"nota,omitempty"`
	Aprobado   *bool      `json:"aprobado,omitempty"`
	IniciadoEn time.Time  `json:"iniciado_en"`
	EnviadoEn  *time.Time `json:"enviado_en,omitempty"`
}

type InsigniaExportada struct {
	Codigo     string     `json:"codigo_de_verificacion"`
	CursoID    uuid.UUID  `json:"curso_id"`
	EmitidaEn  time.Time  `json:"emitida_en"`
	RevocadaEn *time.Time `json:"revocada_en,omitempty"`
}

type AccionAuditada struct {
	Accion     string          `json:"accion"`
	Entidad    string          `json:"entidad"`
	Metadata   json.RawMessage `json:"metadata,omitempty"`
	OcurridaEn time.Time       `json:"ocurrida_en"`
}

// DeUsuario reúne los datos de una persona.
func (r *ExportacionRepo) DeUsuario(ctx context.Context, userID uuid.UUID) (*DatosPersonales, error) {
	out := &DatosPersonales{GeneradoEn: time.Now().UTC()}

	if err := r.pool.QueryRow(ctx, `
		SELECT id, email, full_name, role, status, email_verified_at, created_at
		  FROM users WHERE id = $1`, userID).
		Scan(&out.Cuenta.ID, &out.Cuenta.Email, &out.Cuenta.NombreCompleto, &out.Cuenta.Rol,
			&out.Cuenta.Estado, &out.Cuenta.CorreoVerifadoEn, &out.Cuenta.CreadaEn); err != nil {
		return nil, err
	}

	var err error
	if out.Sesiones, err = r.sesiones(ctx, userID); err != nil {
		return nil, err
	}
	if out.Inscripciones, err = r.inscripciones(ctx, userID); err != nil {
		return nil, err
	}
	if out.Insignias, err = r.insignias(ctx, userID); err != nil {
		return nil, err
	}
	if out.Auditoria, err = r.auditoria(ctx, userID); err != nil {
		return nil, err
	}
	return out, nil
}

func (r *ExportacionRepo) sesiones(ctx context.Context, userID uuid.UUID) ([]SesionResumida, error) {
	filas, err := r.pool.Query(ctx, `
		SELECT created_at, expires_at, revoked_at, coalesce(ip_address, ''), coalesce(user_agent, '')
		  FROM sessions WHERE user_id = $1 ORDER BY created_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer filas.Close()

	out := []SesionResumida{}
	for filas.Next() {
		var s SesionResumida
		if err := filas.Scan(&s.CreadaEn, &s.VenceEn, &s.RevocadaEn, &s.IP, &s.UserAgent); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, filas.Err()
}

func (r *ExportacionRepo) inscripciones(ctx context.Context, userID uuid.UUID) ([]InscripcionExportada, error) {
	filas, err := r.pool.Query(ctx, `
		SELECT e.id, e.course_id, coalesce(v.title, ''), e.status,
		       e.enrolled_at, e.completed_at, e.approved_at
		  FROM enrollments e
		  LEFT JOIN course_versions v ON v.id = e.course_version_id
		 WHERE e.student_id = $1
		 ORDER BY e.enrolled_at`, userID)
	if err != nil {
		return nil, err
	}

	type fila struct {
		id  uuid.UUID
		exp InscripcionExportada
	}
	var crudas []fila
	for filas.Next() {
		var f fila
		if err := filas.Scan(&f.id, &f.exp.CursoID, &f.exp.CursoTitulo, &f.exp.Estado,
			&f.exp.InscritoEn, &f.exp.CompletadoEn, &f.exp.AprobadoEn); err != nil {
			filas.Close()
			return nil, err
		}
		crudas = append(crudas, f)
	}
	filas.Close()
	if err := filas.Err(); err != nil {
		return nil, err
	}

	out := []InscripcionExportada{}
	for _, f := range crudas {
		progreso, err := r.progresoDe(ctx, f.id)
		if err != nil {
			return nil, err
		}
		intentos, err := r.intentosDe(ctx, f.id)
		if err != nil {
			return nil, err
		}
		f.exp.Progreso = progreso
		f.exp.Intentos = intentos
		out = append(out, f.exp)
	}
	return out, nil
}

func (r *ExportacionRepo) progresoDe(ctx context.Context, enrollmentID uuid.UUID) ([]ProgresoExportado, error) {
	filas, err := r.pool.Query(ctx, `
		SELECT resource_stable_id, status, time_spent_seconds, first_opened_at, completed_at
		  FROM resource_progress WHERE enrollment_id = $1`, enrollmentID)
	if err != nil {
		return nil, err
	}
	defer filas.Close()

	out := []ProgresoExportado{}
	for filas.Next() {
		var p ProgresoExportado
		if err := filas.Scan(&p.RecursoStableID, &p.Estado, &p.TiempoSegundos,
			&p.PrimeraAperturaEn, &p.CompletadoEn); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, filas.Err()
}

func (r *ExportacionRepo) intentosDe(ctx context.Context, enrollmentID uuid.UUID) ([]IntentoExportado, error) {
	filas, err := r.pool.Query(ctx, `
		SELECT attempt_number, status, score, passed, started_at, submitted_at
		  FROM quiz_attempts WHERE enrollment_id = $1 ORDER BY started_at`, enrollmentID)
	if err != nil {
		return nil, err
	}
	defer filas.Close()

	out := []IntentoExportado{}
	for filas.Next() {
		var i IntentoExportado
		if err := filas.Scan(&i.Numero, &i.Estado, &i.Nota, &i.Aprobado, &i.IniciadoEn, &i.EnviadoEn); err != nil {
			return nil, err
		}
		out = append(out, i)
	}
	return out, filas.Err()
}

func (r *ExportacionRepo) insignias(ctx context.Context, userID uuid.UUID) ([]InsigniaExportada, error) {
	filas, err := r.pool.Query(ctx, `
		SELECT verification_code, course_id, issued_at, revoked_at
		  FROM badges WHERE student_id = $1 ORDER BY issued_at`, userID)
	if err != nil {
		return nil, err
	}
	defer filas.Close()

	out := []InsigniaExportada{}
	for filas.Next() {
		var b InsigniaExportada
		if err := filas.Scan(&b.Codigo, &b.CursoID, &b.EmitidaEn, &b.RevocadaEn); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, filas.Err()
}

// auditoria devuelve lo registrado sobre esta cuenta, sea quien sea quien lo
// hizo: para la persona, "me suspendieron la cuenta" es un dato suyo aunque la
// acción la ejecutara otro.
func (r *ExportacionRepo) auditoria(ctx context.Context, userID uuid.UUID) ([]AccionAuditada, error) {
	filas, err := r.pool.Query(ctx, `
		SELECT action, entity_type, metadata, created_at
		  FROM audit_logs
		 WHERE actor_user_id = $1 OR entity_id = $1::text
		 ORDER BY created_at DESC
		 LIMIT 1000`, userID)
	if err != nil {
		return nil, err
	}
	defer filas.Close()

	out := []AccionAuditada{}
	for filas.Next() {
		var a AccionAuditada
		var meta []byte
		if err := filas.Scan(&a.Accion, &a.Entidad, &meta, &a.OcurridaEn); err != nil {
			return nil, err
		}
		a.Metadata = json.RawMessage(meta)
		out = append(out, a)
	}
	return out, filas.Err()
}
