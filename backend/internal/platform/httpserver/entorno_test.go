package httpserver_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"github.com/DES-SOLUCIONES-CLOUD/proyecto-1/backend/internal/app/admin"
	"github.com/DES-SOLUCIONES-CLOUD/proyecto-1/backend/internal/app/auth"
	"github.com/DES-SOLUCIONES-CLOUD/proyecto-1/backend/internal/app/courses"
	"github.com/DES-SOLUCIONES-CLOUD/proyecto-1/backend/internal/app/enrollments"
	"github.com/DES-SOLUCIONES-CLOUD/proyecto-1/backend/internal/app/progreso"
	"github.com/DES-SOLUCIONES-CLOUD/proyecto-1/backend/internal/app/quizzes"
	"github.com/DES-SOLUCIONES-CLOUD/proyecto-1/backend/internal/domain/user"
	"github.com/DES-SOLUCIONES-CLOUD/proyecto-1/backend/internal/platform/httpserver"
	"github.com/DES-SOLUCIONES-CLOUD/proyecto-1/backend/internal/platform/mailer"
	"github.com/DES-SOLUCIONES-CLOUD/proyecto-1/backend/internal/platform/postgres"
	"github.com/DES-SOLUCIONES-CLOUD/proyecto-1/backend/internal/platform/redisclient"
	"github.com/DES-SOLUCIONES-CLOUD/proyecto-1/backend/internal/platform/storage"
	"github.com/DES-SOLUCIONES-CLOUD/proyecto-1/backend/migrations"
)

// Estas pruebas ejercen la API de identidad contra PostgreSQL y Redis reales:
// lo que verifican (unicidad, consumo atómico de tokens, revocación
// inmediata, límites de tasa, inmutabilidad de la auditoría) vive en esos
// adaptadores y un doble de prueba no lo demostraría.
//
// Se omiten si falta infraestructura:
//
//	TEST_DATABASE_URL=postgres://mooc:mooc@localhost:5432/mooc?sslmode=disable \
//	TEST_REDIS_ADDR=localhost:6379 go test ./...

// buzon captura los correos salientes. Hace de Mailpit: las pruebas leen de
// aquí el enlace que recibiría el usuario, porque la API nunca lo devuelve.
type buzon struct {
	mu       sync.Mutex
	mensajes []mensaje
}

type mensaje struct{ Para, Asunto, Cuerpo string }

func (b *buzon) Send(to, subject, body string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.mensajes = append(b.mensajes, mensaje{Para: to, Asunto: subject, Cuerpo: body})
	return nil
}

func (b *buzon) total() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.mensajes)
}

// tokenPara extrae el token del último correo enviado a esa dirección.
func (b *buzon) tokenPara(t *testing.T, destino string) string {
	t.Helper()
	b.mu.Lock()
	defer b.mu.Unlock()
	for i := len(b.mensajes) - 1; i >= 0; i-- {
		if b.mensajes[i].Para != destino {
			continue
		}
		_, tras, ok := strings.Cut(b.mensajes[i].Cuerpo, "?token=")
		if !ok {
			t.Fatalf("el correo a %s no traía token: %s", destino, b.mensajes[i].Cuerpo)
		}
		token, _, _ := strings.Cut(tras, "\n")
		return strings.TrimSpace(token)
	}
	t.Fatalf("no se envió ningún correo a %s", destino)
	return ""
}

// entregaPorCDN hace de almacenamiento servido tras un CDN. Las pruebas de
// reproducción verifican la autorización, no la firma de objetos, así que no
// necesitan MinIO levantado.
type entregaPorCDN struct{ base string }

func (e entregaPorCDN) PresignedGetURL(_ context.Context, objectKey string, _ time.Duration, _ string) (string, error) {
	return e.base + "/" + objectKey, nil
}

func (e entregaPorCDN) SirveDesdeCDN() bool { return true }

// almacenEnMemoria hace de almacén de objetos para las pruebas de carga. El
// cliente real contacta a MinIO al construirse, así que sin un doble no hay
// forma de probar el ciclo de vida de una carga multipart —ni de notar que un
// handler desreferencia un almacén ausente.
//
// Guarda lo justo para que las aserciones sean sobre comportamiento y no sobre
// llamadas: qué partes se subieron, qué se cerró y qué se borró.
type almacenEnMemoria struct {
	mu        sync.Mutex
	objetos   map[string][]byte
	cargas    map[string][]storage.ParteEnCurso // uploadID -> partes
	borrados  []string
	contenido string // MIME que DetectMIME devolverá
	siguiente int
}

func nuevoAlmacenEnMemoria() *almacenEnMemoria {
	return &almacenEnMemoria{
		objetos: map[string][]byte{},
		cargas:  map[string][]storage.ParteEnCurso{},
	}
}

func (a *almacenEnMemoria) PresignedPutURL(_ context.Context, objectKey string, _ time.Duration) (string, error) {
	return "https://almacen.pruebas.local/" + objectKey + "?firma=puesta", nil
}

func (a *almacenEnMemoria) InitiateMultipartUpload(_ context.Context, objectKey, contentType string) (string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.siguiente++
	id := fmt.Sprintf("carga-%d", a.siguiente)
	a.cargas[id] = nil
	if contentType != "" {
		a.contenido = contentType
	}
	return id, nil
}

// PresignedUploadPartURL firma la parte y, para que la prueba pueda reanudar,
// la da por subida: el cliente real haría el PUT contra esa URL.
func (a *almacenEnMemoria) PresignedUploadPartURL(_ context.Context, objectKey, uploadID string, partNumber int, _ time.Duration) (string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if _, ok := a.cargas[uploadID]; !ok {
		return "", errors.New("carga desconocida")
	}
	a.cargas[uploadID] = append(a.cargas[uploadID], storage.ParteEnCurso{
		Numero: partNumber, Tamano: 5 << 20, ETag: fmt.Sprintf("etag-%d", partNumber),
	})
	return fmt.Sprintf("https://almacen.pruebas.local/%s?partNumber=%d&uploadId=%s", objectKey, partNumber, uploadID), nil
}

func (a *almacenEnMemoria) CerrarCargaMultiparte(_ context.Context, objectKey, uploadID string, partes []storage.ParteCargada) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if _, ok := a.cargas[uploadID]; !ok {
		return errors.New("carga desconocida")
	}
	total := 0
	for range partes {
		total += 5 << 20
	}
	a.objetos[objectKey] = make([]byte, total)
	delete(a.cargas, uploadID)
	return nil
}

func (a *almacenEnMemoria) PartesYaSubidas(_ context.Context, _ string, uploadID string) ([]storage.ParteEnCurso, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	partes, ok := a.cargas[uploadID]
	if !ok {
		return nil, errors.New("carga desconocida")
	}
	return partes, nil
}

func (a *almacenEnMemoria) Metadatos(_ context.Context, objectKey string) (storage.ObjetoInfo, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	b, ok := a.objetos[objectKey]
	if !ok {
		return storage.ObjetoInfo{}, errors.New("objeto ausente")
	}
	ct := a.contenido
	if ct == "" {
		ct = "application/octet-stream"
	}
	return storage.ObjetoInfo{Tamano: int64(len(b)), ContentType: ct, ETag: "etag-objeto"}, nil
}

func (a *almacenEnMemoria) DetectMIME(_ context.Context, _ string) (string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.contenido == "" {
		return "application/octet-stream", nil
	}
	return a.contenido, nil
}

func (a *almacenEnMemoria) CalculateSHA256(_ context.Context, objectKey string) (string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	b, ok := a.objetos[objectKey]
	if !ok {
		return "", errors.New("objeto ausente")
	}
	suma := sha256.Sum256(b)
	return hex.EncodeToString(suma[:]), nil
}

func (a *almacenEnMemoria) RemoveObject(_ context.Context, objectKey string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	delete(a.objetos, objectKey)
	a.borrados = append(a.borrados, objectKey)
	return nil
}

// fueBorrado dice si el almacén eliminó el objeto, que es cómo se comprueba
// que un rechazo de integridad o de MIME no deja basura detrás.
func (a *almacenEnMemoria) fueBorrado(objectKey string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	for _, k := range a.borrados {
		if k == objectKey {
			return true
		}
	}
	return false
}

type entorno struct {
	t       *testing.T
	handler http.Handler
	pool    *pgxpool.Pool
	rdb     *redis.Client
	correos *buzon
	almacen *almacenEnMemoria
}

func nuevoEntorno(t *testing.T) *entorno {
	t.Helper()

	urlBase := os.Getenv("TEST_DATABASE_URL")
	dirRedis := os.Getenv("TEST_REDIS_ADDR")
	if urlBase == "" || dirRedis == "" {
		t.Skip("faltan TEST_DATABASE_URL y TEST_REDIS_ADDR: se omiten las pruebas de integración")
	}

	ctx := context.Background()
	pool, err := postgres.Connect(ctx, urlBase)
	if err != nil {
		t.Fatalf("PostgreSQL: %v", err)
	}
	t.Cleanup(pool.Close)

	if err := postgres.Migrate(ctx, pool, migrations.FS); err != nil {
		t.Fatalf("migraciones: %v", err)
	}
	// Cada prueba parte de una base limpia. audit_logs se vacía aparte porque
	// su disparador de inmutabilidad rechaza DELETE pero no TRUNCATE.
	if _, err := pool.Exec(ctx, `TRUNCATE audit_logs, users, courses CASCADE`); err != nil {
		t.Fatalf("limpieza: %v", err)
	}

	rdb, err := redisclient.Connect(ctx, dirRedis)
	if err != nil {
		t.Fatalf("Redis: %v", err)
	}
	t.Cleanup(func() { _ = rdb.Close() })
	if err := rdb.FlushDB(ctx).Err(); err != nil {
		t.Fatalf("limpieza de Redis: %v", err)
	}

	correos := &buzon{}
	users := postgres.NewUserRepo(pool)
	authSvc := auth.NewService(users, mailer.NewWithSender(correos, "pruebas@mooc.local"),
		"http://localhost:3000", user.DefaultSessionTTL)

	cursos := postgres.NewCourseRepo(pool)
	cursosSvc := courses.NewService(cursos)
	inscripciones := postgres.NewEnrollmentRepo(pool)
	avance := postgres.NewProgressRepo(pool)
	evaluaciones := postgres.NewQuizRepo(pool)
	insignias := postgres.NewBadgeRepo(pool)

	almacen := nuevoAlmacenEnMemoria()
	progresoSvc := progreso.NewService(avance, inscripciones, cursos, evaluaciones, insignias, users)
	handler := httpserver.NewRouter(httpserver.Deps{
		Auth:         authSvc,
		Admin:        admin.NewService(users),
		Courses:      cursosSvc,
		Enrollments:  enrollments.NewService(inscripciones, cursos, avance),
		Quizzes:      quizzes.NewService(evaluaciones, cursos, cursosSvc, inscripciones, progresoSvc),
		Progreso:     progresoSvc,
		Entrega:      entregaPorCDN{base: "https://cdn.pruebas.local"},
		Storage:      almacen,
		Redis:        rdb,
		CORSOrigin:   "http://localhost:3000",
		CookieSecure: false,
	})
	return &entorno{t: t, handler: handler, pool: pool, rdb: rdb, correos: correos, almacen: almacen}
}

// cliente conserva cookies entre peticiones, como un navegador, y reenvía el
// token anti-CSRF en la cabecera.
type cliente struct {
	env     *entorno
	cookies map[string]string
}

func (e *entorno) cliente() *cliente {
	return &cliente{env: e, cookies: map[string]string{}}
}

type respuesta struct {
	Estado   int
	Cuerpo   map[string]any
	Crudo    string
	Cabecera http.Header
}

func (r respuesta) campo(t *testing.T, ruta ...string) any {
	t.Helper()
	var actual any = r.Cuerpo
	for _, paso := range ruta {
		mapa, ok := actual.(map[string]any)
		if !ok {
			t.Fatalf("no se pudo navegar hasta %v en %s", ruta, r.Crudo)
		}
		actual = mapa[paso]
	}
	return actual
}

func (c *cliente) hacer(metodo, ruta string, cuerpo any, cabeceras ...[2]string) respuesta {
	c.env.t.Helper()

	var lector io.Reader
	if cuerpo != nil {
		crudo, err := json.Marshal(cuerpo)
		if err != nil {
			c.env.t.Fatalf("cuerpo no serializable: %v", err)
		}
		lector = bytes.NewReader(crudo)
	}

	req := httptest.NewRequest(metodo, "/api/v1"+ruta, lector)
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "192.0.2.10:1234"
	for nombre, valor := range c.cookies {
		req.AddCookie(&http.Cookie{Name: nombre, Value: valor})
	}
	if csrf, ok := c.cookies[httpserver.CSRFCookieName]; ok {
		req.Header.Set(httpserver.CSRFHeaderName, csrf)
	}
	for _, h := range cabeceras {
		req.Header.Set(h[0], h[1])
	}

	grabadora := httptest.NewRecorder()
	c.env.handler.ServeHTTP(grabadora, req)

	res := grabadora.Result()
	defer res.Body.Close()
	for _, ck := range res.Cookies() {
		if ck.MaxAge < 0 {
			delete(c.cookies, ck.Name)
			continue
		}
		c.cookies[ck.Name] = ck.Value
	}

	crudo, _ := io.ReadAll(res.Body)
	salida := respuesta{Estado: res.StatusCode, Crudo: string(crudo), Cabecera: res.Header}
	if len(crudo) > 0 {
		_ = json.Unmarshal(crudo, &salida.Cuerpo)
	}
	return salida
}

const (
	clavePrueba = "Clave-De-Prueba-1"
	claveNueva  = "Clave-Nueva-Segura-2"
)

// registrarYVerificar deja una cuenta lista para iniciar sesión.
func (e *entorno) registrarYVerificar(correo string) {
	e.t.Helper()
	c := e.cliente()
	res := c.hacer(http.MethodPost, "/auth/register", map[string]string{
		"email": correo, "password": clavePrueba, "full_name": "Persona de prueba",
	})
	if res.Estado != http.StatusAccepted {
		e.t.Fatalf("registro: %d %s", res.Estado, res.Crudo)
	}
	token := e.correos.tokenPara(e.t, correo)
	if res := c.hacer(http.MethodPost, "/auth/verify-email", map[string]string{"token": token}); res.Estado != http.StatusOK {
		e.t.Fatalf("verificación: %d %s", res.Estado, res.Crudo)
	}
}

// entrar inicia sesión y devuelve el cliente con sus cookies.
func (e *entorno) entrar(correo, clave string) *cliente {
	e.t.Helper()
	c := e.cliente()
	res := c.hacer(http.MethodPost, "/auth/login", map[string]string{"email": correo, "password": clave})
	if res.Estado != http.StatusOK {
		e.t.Fatalf("login: %d %s", res.Estado, res.Crudo)
	}
	return c
}

// asciendeA cambia el rol de una cuenta directamente en base. Es el atajo
// para tener un administrador o un profesor, que por diseño no se crean por
// registro público.
func (e *entorno) asciendeA(correo string, rol string) {
	e.t.Helper()
	if _, err := e.pool.Exec(context.Background(),
		`UPDATE users SET role=$2 WHERE email=$1`, correo, rol); err != nil {
		e.t.Fatalf("no se pudo asignar el rol %s a %s: %v", rol, correo, err)
	}
}

// profesorConCurso deja una cuenta de profesor con un curso en borrador, y
// devuelve el cliente autenticado junto con el identificador de la versión.
func (e *entorno) profesorConCurso(correo, slug string) (*cliente, string) {
	e.t.Helper()
	e.registrarYVerificar(correo)
	e.asciendeA(correo, "teacher")
	c := e.entrar(correo, clavePrueba)

	res := c.hacer(http.MethodPost, "/courses", map[string]string{
		"slug": slug, "title": "Curso de prueba",
	})
	if res.Estado != http.StatusCreated {
		e.t.Fatalf("crear curso: %d %s", res.Estado, res.Crudo)
	}
	versionID, _ := res.campo(e.t, "version_id").(string)
	if versionID == "" {
		e.t.Fatalf("no se pudo leer el id de la versión: %s", res.Crudo)
	}
	return c, versionID
}

// consultarTexto devuelve un único valor de texto.
func (e *entorno) consultarTexto(consulta string, args ...any) string {
	e.t.Helper()
	var v string
	if err := e.pool.QueryRow(context.Background(), consulta, args...).Scan(&v); err != nil {
		e.t.Fatalf("consulta %q: %v", consulta, err)
	}
	return v
}

func (e *entorno) contar(consulta string, args ...any) int {
	e.t.Helper()
	var n int
	if err := e.pool.QueryRow(context.Background(), consulta, args...).Scan(&n); err != nil {
		e.t.Fatalf("consulta %q: %v", consulta, err)
	}
	return n
}
