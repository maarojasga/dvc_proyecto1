// Command seed construye el conjunto de datos sintéticos de la prueba de
// carga y de la demostración de aceptación.
//
// La sección 10.1 del enunciado exige que la demostración se ejecute "con
// datos sintéticos sobre el sistema desplegado mediante Docker Compose". Este
// comando produce esos datos por las mismas rutas que usa la aplicación —los
// servicios de dominio, no INSERT a mano—, así que un curso sembrado es
// indistinguible de uno creado por un profesor: pasa las mismas validaciones
// de publicación y tiene los mismos identificadores estables.
//
// Además emite las sesiones que necesita el generador de carga. Eso no es un
// atajo alrededor de la seguridad, es lo contrario: el límite de tasa del
// login es de diez intentos por minuto y por IP, y todo el tráfico de k6 sale
// de una sola IP. Autenticar 500 usuarios por el endpoint tardaría casi una
// hora y mediría el limitador en vez de la plataforma. Sembrar las sesiones
// deja el limitador intacto —la prueba lo ejercita aparte, dentro de su
// presupuesto— y permite medir lo que de verdad importa: el catálogo, la
// entrega de contenido, el progreso y los quizzes.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/google/uuid"

	"github.com/DES-SOLUCIONES-CLOUD/proyecto-1/backend/internal/app/courses"
	"github.com/DES-SOLUCIONES-CLOUD/proyecto-1/backend/internal/app/enrollments"
	"github.com/DES-SOLUCIONES-CLOUD/proyecto-1/backend/internal/app/quizzes"
	"github.com/DES-SOLUCIONES-CLOUD/proyecto-1/backend/internal/config"
	domain "github.com/DES-SOLUCIONES-CLOUD/proyecto-1/backend/internal/domain/course"
	"github.com/DES-SOLUCIONES-CLOUD/proyecto-1/backend/internal/domain/enrollment"
	"github.com/DES-SOLUCIONES-CLOUD/proyecto-1/backend/internal/domain/user"
	"github.com/DES-SOLUCIONES-CLOUD/proyecto-1/backend/internal/platform/postgres"
	"github.com/DES-SOLUCIONES-CLOUD/proyecto-1/backend/internal/platform/security"

	"github.com/DES-SOLUCIONES-CLOUD/proyecto-1/backend/migrations"
)

// Forma del curso sintético. Tres módulos de dos unidades con cuatro recursos
// de texto y un quiz dan 27 recursos, de los que 15 son obligatorios: bastante
// para que el cálculo de progreso sea real y poco para que sembrar 500
// estudiantes no tarde más que la propia prueba.
const (
	modulos          = 3
	unidadesPorMod   = 2
	recursosPorUnid  = 4
	preguntasPorQuiz = 5

	slugDelCurso = "carga-etapa-1"
	dominio      = "carga.mooc.local"

	// Contraseña de las cuentas sintéticas. Es pública a propósito: este
	// comando no debe correr jamás contra un entorno real, y una contraseña
	// que parezca secreta invitaría justo a eso.
	claveSintetica = "carga-etapa-1-sintetica"
)

// Salida es lo que el generador de carga necesita saber del entorno sembrado.
type Salida struct {
	GeneradoEn     time.Time  `json:"generado_en"`
	CourseID       string     `json:"course_id"`
	VersionID      string     `json:"version_id"`
	RecursosTexto  []string   `json:"recursos_texto"`
	RecursosQuiz   []string   `json:"recursos_quiz"`
	Estudiantes    []Cuenta   `json:"estudiantes"`
	CuentaDePrueba Credencial `json:"cuenta_de_prueba"`
}

// Cuenta es un estudiante inscrito con su sesión ya abierta.
type Cuenta struct {
	Email string `json:"email"`
	Token string `json:"token"`
}

// Credencial son unas credenciales completas, para el escenario que sí mide
// el login por la puerta principal.
type Credencial struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func main() {
	if err := ejecutar(); err != nil {
		log.Fatalf("seed: %v", err)
	}
}

func ejecutar() error {
	cfg := config.Load()
	ctx := context.Background()

	estudiantes := enteroDelEntorno("SEED_STUDENTS", 500)
	destino := os.Getenv("SEED_OUT")
	if destino == "" {
		destino = "/salida/escenario.json"
	}

	pool, err := postgres.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("postgres: %w", err)
	}
	defer pool.Close()

	// Se migra aquí también: el sembrador puede ganarle el arranque a la API,
	// y fallar por una tabla que no existe todavía sería un falso negativo.
	if err := postgres.Migrate(ctx, pool, migrations.FS); err != nil {
		return fmt.Errorf("migraciones: %w", err)
	}

	users := postgres.NewUserRepo(pool)
	courseRepo := postgres.NewCourseRepo(pool)
	enrollmentRepo := postgres.NewEnrollmentRepo(pool)
	progressRepo := postgres.NewProgressRepo(pool)
	quizRepo := postgres.NewQuizRepo(pool)
	iframeRepo := postgres.NewIframeRepo(pool)

	coursesSvc := courses.NewService(courseRepo, iframeRepo, postgres.NewColaboradoresRepo(pool))
	enrollmentsSvc := enrollments.NewService(enrollmentRepo, courseRepo, progressRepo, iframeRepo)
	quizzesSvc := quizzes.NewService(quizRepo, courseRepo, coursesSvc, enrollmentRepo, nil)

	profesor, err := cuenta(ctx, users, "profesor@"+dominio, "Profesora de carga", user.RoleTeacher)
	if err != nil {
		return fmt.Errorf("profesor: %w", err)
	}

	salida := Salida{
		GeneradoEn:     time.Now().UTC(),
		CuentaDePrueba: Credencial{Email: "sonda@" + dominio, Password: claveSintetica},
	}

	if err := construirCurso(ctx, coursesSvc, quizzesSvc, courseRepo, profesor, &salida); err != nil {
		return err
	}

	// La cuenta sonda existe para el escenario que mide el login real. Va
	// aparte de las demás porque es la única que necesita contraseña utilizable.
	if _, err := cuenta(ctx, users, salida.CuentaDePrueba.Email, "Sonda de carga", user.RoleStudent); err != nil {
		return fmt.Errorf("cuenta sonda: %w", err)
	}

	cursoID := uuid.MustParse(salida.CourseID)
	for i := 0; i < estudiantes; i++ {
		email := fmt.Sprintf("estudiante-%04d@%s", i, dominio)
		u, err := cuenta(ctx, users, email, fmt.Sprintf("Estudiante %04d", i), user.RoleStudent)
		if err != nil {
			return fmt.Errorf("estudiante %d: %w", i, err)
		}
		if _, err := enrollmentsSvc.Enroll(ctx, u, cursoID); err != nil && !yaInscrito(err) {
			return fmt.Errorf("inscripción de %s: %w", email, err)
		}
		token, err := abrirSesion(ctx, users, u, cfg.SessionTTL)
		if err != nil {
			return fmt.Errorf("sesión de %s: %w", email, err)
		}
		salida.Estudiantes = append(salida.Estudiantes, Cuenta{Email: email, Token: token})

		if (i+1)%100 == 0 {
			log.Printf("seed: %d/%d estudiantes", i+1, estudiantes)
		}
	}

	if err := escribir(destino, salida); err != nil {
		return err
	}
	log.Printf("seed: %d estudiantes inscritos en %s; escenario en %s",
		len(salida.Estudiantes), salida.CourseID, destino)
	return nil
}

// construirCurso crea el curso sintético y lo publica, o reutiliza el que ya
// exista. Reutilizarlo importa: sembrar es idempotente, así que repetir la
// prueba de carga no multiplica cursos ni invalida las inscripciones previas.
func construirCurso(
	ctx context.Context,
	coursesSvc *courses.Service,
	quizzesSvc *quizzes.Service,
	courseRepo *postgres.CourseRepo,
	profesor *user.User,
	salida *Salida,
) error {
	if c, err := courseRepo.GetCourseBySlug(ctx, slugDelCurso); err == nil && c.CurrentPublishedVersionID != nil {
		v, err := courseRepo.GetVersion(ctx, *c.CurrentPublishedVersionID)
		if err != nil {
			return fmt.Errorf("versión publicada: %w", err)
		}
		salida.CourseID = c.ID.String()
		salida.VersionID = v.ID.String()
		clasificarRecursos(v, salida)
		log.Printf("seed: reutilizando el curso %s ya publicado", slugDelCurso)
		return nil
	}

	_, version, err := coursesSvc.CreateDraft(ctx, profesor, slugDelCurso, "Fundamentos de cloud (carga)")
	if err != nil {
		return fmt.Errorf("borrador: %w", err)
	}

	if err := coursesSvc.UpdateMetadata(ctx, profesor, version.ID, domain.Version{
		Title:         "Fundamentos de cloud (carga)",
		Summary:       "Curso sintético para la prueba de carga de Etapa 1.",
		DescriptionMD: "Generado por `cmd/seed`. No es material didáctico real.",
		Category:      "Infraestructura",
		Level:         "Introductorio",
		Language:      "es",
		// Los umbrales de aprobación se dejan exigentes para que el escenario
		// de quiz produzca aprobados y reprobados, y el cálculo de progreso
		// recorra las dos ramas.
		ApprovalMinScore:             60,
		ApprovalRequiredResourcesPct: 80,
	}); err != nil {
		return fmt.Errorf("metadatos: %w", err)
	}

	for m := 1; m <= modulos; m++ {
		modulo, err := coursesSvc.AddModule(ctx, profesor, version.ID, fmt.Sprintf("Módulo %d", m), m)
		if err != nil {
			return fmt.Errorf("módulo %d: %w", m, err)
		}
		// La evaluación del módulo cuelga de su última unidad, que es donde
		// la pondría un profesor: se evalúa lo que se acaba de ver.
		var ultimaUnidad uuid.UUID
		for u := 1; u <= unidadesPorMod; u++ {
			unidad, err := coursesSvc.AddUnit(ctx, profesor, version.ID, modulo.ID, fmt.Sprintf("Unidad %d.%d", m, u), u)
			if err != nil {
				return fmt.Errorf("unidad %d.%d: %w", m, u, err)
			}
			ultimaUnidad = unidad.ID
			for r := 1; r <= recursosPorUnid; r++ {
				// Solo recursos de texto: un binario exigiría al worker
				// transcodificar antes de poder publicar, y lo que esta
				// prueba mide es la API, no FFmpeg. La carga multimedia se
				// ejercita en su propio segmento de la demostración.
				res, err := coursesSvc.AddResource(ctx, profesor, version.ID, &domain.Resource{
					UnitID: unidad.ID, Type: domain.ResourceText,
					Title:    fmt.Sprintf("Lección %d.%d.%d", m, u, r),
					Position: r, Visible: true,
					// Uno de cada dos es obligatorio: así el porcentaje de
					// avance no es trivialmente 100 ni 0.
					Required:      r%2 == 1,
					TextContentMD: leccion(m, u, r),
				})
				if err != nil {
					return fmt.Errorf("recurso %d.%d.%d: %w", m, u, r, err)
				}
				salida.RecursosTexto = append(salida.RecursosTexto, res.ID.String())
			}
		}

		quizRes, err := coursesSvc.AddResource(ctx, profesor, version.ID, &domain.Resource{
			UnitID: ultimaUnidad, Type: domain.ResourceQuiz,
			Title:    fmt.Sprintf("Evaluación del módulo %d", m),
			Position: recursosPorUnid + 1, Visible: true, Required: true,
		})
		if err != nil {
			return fmt.Errorf("evaluación del módulo %d: %w", m, err)
		}
		if _, err := quizzesSvc.Definir(ctx, profesor, version.ID, quizRes.ID, definicionDeQuiz(m)); err != nil {
			return fmt.Errorf("definición del quiz %d: %w", m, err)
		}
		salida.RecursosQuiz = append(salida.RecursosQuiz, quizRes.ID.String())
	}

	if err := coursesSvc.PublishVersion(ctx, profesor, version.ID); err != nil {
		return fmt.Errorf("publicación: %w", err)
	}
	salida.CourseID = version.CourseID.String()
	salida.VersionID = version.ID.String()
	return nil
}

// clasificarRecursos separa los identificadores por tipo para el generador de
// carga, que ataca los de texto y los de quiz con escenarios distintos.
func clasificarRecursos(v *domain.Version, salida *Salida) {
	for _, m := range v.Modules {
		for _, u := range m.Units {
			for _, r := range u.Resources {
				switch r.Type {
				case domain.ResourceQuiz:
					salida.RecursosQuiz = append(salida.RecursosQuiz, r.ID.String())
				case domain.ResourceText:
					salida.RecursosTexto = append(salida.RecursosTexto, r.ID.String())
				}
			}
		}
	}
}

func definicionDeQuiz(modulo int) quizzes.DefinicionQuiz {
	def := quizzes.DefinicionQuiz{
		Title:            fmt.Sprintf("Evaluación del módulo %d", modulo),
		PassScore:        60,
		FeedbackPolicy:   "after_submit",
		ShuffleQuestions: true,
	}
	for p := 1; p <= preguntasPorQuiz; p++ {
		def.Questions = append(def.Questions, quizzes.DefinicionPregunta{
			PromptMD: fmt.Sprintf("Pregunta %d del módulo %d", p, modulo),
			Type:     "single",
			Points:   1,
			Options: []quizzes.DefinicionOpcion{
				{TextMD: "Opción correcta", IsCorrect: true},
				{TextMD: "Distractor A"},
				{TextMD: "Distractor B"},
				{TextMD: "Distractor C"},
			},
		})
	}
	return def
}

func leccion(m, u, r int) string {
	return fmt.Sprintf("## Lección %d.%d.%d\n\nContenido sintético para la prueba de carga.\n\n"+
		"- Punto uno\n- Punto dos\n- Punto tres\n", m, u, r)
}

// cuenta crea la cuenta si no existe y la devuelve activa y verificada. Es
// idempotente porque sembrar dos veces es normal: se repite la prueba.
func cuenta(ctx context.Context, users *postgres.UserRepo, email, nombre string, rol user.Role) (*user.User, error) {
	if u, err := users.GetByEmail(ctx, email); err == nil {
		return u, nil
	} else if !errors.Is(err, postgres.ErrNotFound) {
		return nil, err
	}

	hash, err := security.HashPassword(claveSintetica)
	if err != nil {
		return nil, err
	}
	ahora := time.Now().UTC()
	u := &user.User{
		ID: uuid.New(), Email: email, PasswordHash: hash, FullName: nombre,
		Role: rol, Status: user.StatusActive, EmailVerifiedAt: &ahora,
		CreatedAt: ahora, UpdatedAt: ahora,
	}
	if err := users.Create(ctx, u); err != nil {
		return nil, err
	}
	return u, nil
}

// abrirSesion crea una sesión revocable por la misma vía que el login y
// devuelve el token en claro, que es lo único que no vuelve a estar disponible
// después (en la base solo queda su hash).
func abrirSesion(ctx context.Context, users *postgres.UserRepo, u *user.User, ttl time.Duration) (string, error) {
	plano, err := security.GenerateOpaqueToken()
	if err != nil {
		return "", err
	}
	ahora := time.Now().UTC()
	return plano, users.CreateSession(ctx, &user.Session{
		ID: uuid.New(), UserID: u.ID, TokenHash: security.HashToken(plano),
		UserAgent: "k6/carga-etapa-1", IPAddress: "127.0.0.1",
		CreatedAt: ahora, ExpiresAt: ahora.Add(ttl),
	})
}

// yaInscrito distingue "sembrado dos veces" de un fallo real: repetir la
// siembra tiene que ser inofensivo.
func yaInscrito(err error) bool {
	return errors.Is(err, enrollment.ErrAlreadyEnrolled)
}

func escribir(destino string, s Salida) error {
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(destino, b, 0o644)
}

func enteroDelEntorno(clave string, porDefecto int) int {
	if v := os.Getenv(clave); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return porDefecto
}
