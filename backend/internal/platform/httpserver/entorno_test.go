package httpserver_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"github.com/DES-SOLUCIONES-CLOUD/proyecto-1/backend/internal/domain/user"
	"github.com/DES-SOLUCIONES-CLOUD/proyecto-1/backend/internal/platform/httpserver"
	"github.com/DES-SOLUCIONES-CLOUD/proyecto-1/backend/internal/platform/passwords"
	"github.com/DES-SOLUCIONES-CLOUD/proyecto-1/backend/internal/platform/postgres"
	"github.com/DES-SOLUCIONES-CLOUD/proyecto-1/backend/internal/platform/redisclient"
)

// Las pruebas de este paquete ejercen la API completa contra PostgreSQL y
// Redis reales, porque lo que se quiere verificar (unicidad, atomicidad de los
// tokens, revocacion inmediata, limites de tasa) vive precisamente en esos
// adaptadores y un doble de prueba no lo demostraria.
//
// Se omiten si no hay infraestructura configurada:
//
//	TEST_DATABASE_URL=postgres://mooc:mooc@localhost:5432/mooc?sslmode=disable \
//	TEST_REDIS_ADDR=localhost:6379 go test ./...

// notificadorFalso captura los correos en memoria. Hace de Mailpit: las
// pruebas leen de aqui el token que el usuario recibiria por correo, porque
// la API nunca lo devuelve en la respuesta.
type notificadorFalso struct {
	mu           sync.Mutex
	verificacion map[string]string
	recuperacion map[string]string
	entregados   int
}

func nuevoNotificadorFalso() *notificadorFalso {
	return &notificadorFalso{
		verificacion: map[string]string{},
		recuperacion: map[string]string{},
	}
}

func (n *notificadorFalso) EnviarVerificacionCorreo(_ context.Context, destino, _, token string) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.verificacion[destino] = token
	n.entregados++
	return nil
}

func (n *notificadorFalso) EnviarRecuperacionClave(_ context.Context, destino, _, token string) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.recuperacion[destino] = token
	n.entregados++
	return nil
}

func (n *notificadorFalso) tokenVerificacion(t *testing.T, destino string) string {
	t.Helper()
	n.mu.Lock()
	defer n.mu.Unlock()
	token, ok := n.verificacion[destino]
	if !ok {
		t.Fatalf("no se envio correo de verificacion a %s", destino)
	}
	return token
}

func (n *notificadorFalso) tokenRecuperacion(t *testing.T, destino string) string {
	t.Helper()
	n.mu.Lock()
	defer n.mu.Unlock()
	token, ok := n.recuperacion[destino]
	if !ok {
		t.Fatalf("no se envio correo de recuperacion a %s", destino)
	}
	return token
}

func (n *notificadorFalso) totalEntregados() int {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.entregados
}

// entorno es la API montada sobre infraestructura real.
type entorno struct {
	t       *testing.T
	handler http.Handler
	pool    *pgxpool.Pool
	redis   *redis.Client
	correos *notificadorFalso
}

func nuevoEntorno(t *testing.T) *entorno {
	t.Helper()

	urlBase := os.Getenv("TEST_DATABASE_URL")
	dirRedis := os.Getenv("TEST_REDIS_ADDR")
	if urlBase == "" || dirRedis == "" {
		t.Skip("faltan TEST_DATABASE_URL y TEST_REDIS_ADDR: se omiten las pruebas de integracion")
	}

	ctx := context.Background()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))

	pool, err := postgres.Abrir(ctx, urlBase)
	if err != nil {
		t.Fatalf("no se pudo abrir PostgreSQL: %v", err)
	}
	t.Cleanup(pool.Close)

	if err := postgres.Migrar(ctx, pool, nil); err != nil {
		t.Fatalf("no se pudieron aplicar las migraciones: %v", err)
	}
	// Cada prueba parte de una base limpia. CASCADE arrastra roles, sesiones
	// y tokens; la auditoria se vacia aparte porque no cuelga de usuarios.
	if _, err := pool.Exec(ctx, `TRUNCATE auditoria, usuarios CASCADE`); err != nil {
		t.Fatalf("no se pudo limpiar la base: %v", err)
	}

	rdb, err := redisclient.Abrir(ctx, dirRedis)
	if err != nil {
		t.Fatalf("no se pudo abrir Redis: %v", err)
	}
	t.Cleanup(func() { _ = rdb.Close() })
	if err := rdb.FlushDB(ctx).Err(); err != nil {
		t.Fatalf("no se pudo limpiar Redis: %v", err)
	}

	correos := nuevoNotificadorFalso()
	servicio, err := user.NuevoServicio(user.Dependencias{
		Usuarios:  postgres.NuevoRepositorioUsuarios(pool),
		Sesiones:  postgres.NuevoRepositorioSesiones(pool),
		Tokens:    postgres.NuevoRepositorioTokens(pool),
		Auditoria: postgres.NuevoAuditor(pool, log),
		Claves:    passwords.Nuevo(passwords.ParametrosDePrueba()),
		Correos:   correos,
		Cache:     redisclient.NuevaCacheSesiones(rdb, log),
	})
	if err != nil {
		t.Fatalf("no se pudo construir el servicio: %v", err)
	}

	api := httpserver.Nuevo(httpserver.Opciones{
		Usuarios:     servicio,
		Log:          log,
		Limitador:    redisclient.NuevoLimitadorTasa(rdb),
		Idempotencia: redisclient.NuevoAlmacenIdempotencia(rdb),
		CookieSegura: false,
	})

	return &entorno{t: t, handler: api.Handler(), pool: pool, redis: rdb, correos: correos}
}

// cliente conserva las cookies entre peticiones, como haria un navegador, y
// reenvia el token anti-CSRF en la cabecera.
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

// json extrae un campo anidado del cuerpo, para aligerar las aserciones.
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
			c.env.t.Fatalf("no se pudo serializar el cuerpo: %v", err)
		}
		lector = bytes.NewReader(crudo)
	}

	req := httptest.NewRequest(metodo, httpserver.Prefijo+ruta, lector)
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "192.0.2.10:1234"
	for nombre, valor := range c.cookies {
		req.AddCookie(&http.Cookie{Name: nombre, Value: valor})
	}
	// El navegador lee la cookie CSRF y la repite en la cabecera.
	if csrf, ok := c.cookies[httpserver.CookieCSRF]; ok {
		req.Header.Set(httpserver.CabeceraCSRF, csrf)
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

// registrarYActivar deja una cuenta lista para iniciar sesion.
func (e *entorno) registrarYActivar(correo, clave string) {
	e.t.Helper()
	c := e.cliente()
	res := c.hacer(http.MethodPost, "/auth/registro", map[string]string{
		"correo": correo, "nombre": "Persona de prueba", "clave": clave,
	})
	if res.Estado != http.StatusAccepted {
		e.t.Fatalf("registro: estado %d, cuerpo %s", res.Estado, res.Crudo)
	}
	token := e.correos.tokenVerificacion(e.t, correo)
	res = c.hacer(http.MethodPost, "/auth/verificar-correo", map[string]string{"token": token})
	if res.Estado != http.StatusOK {
		e.t.Fatalf("verificacion: estado %d, cuerpo %s", res.Estado, res.Crudo)
	}
}

// entrar inicia sesion y devuelve el cliente con las cookies puestas.
func (e *entorno) entrar(correo, clave string) *cliente {
	e.t.Helper()
	c := e.cliente()
	res := c.hacer(http.MethodPost, "/auth/login", map[string]string{"correo": correo, "clave": clave})
	if res.Estado != http.StatusOK {
		e.t.Fatalf("login: estado %d, cuerpo %s", res.Estado, res.Crudo)
	}
	return c
}

// contar ejecuta un COUNT y devuelve el resultado.
func (e *entorno) contar(consulta string, args ...any) int {
	e.t.Helper()
	var n int
	if err := e.pool.QueryRow(context.Background(), consulta, args...).Scan(&n); err != nil {
		e.t.Fatalf("consulta %q: %v", consulta, err)
	}
	return n
}
