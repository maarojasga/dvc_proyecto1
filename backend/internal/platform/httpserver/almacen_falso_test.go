package httpserver_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/DES-SOLUCIONES-CLOUD/proyecto-1/backend/internal/platform/storage"
)

// almacenFalso es un almacenamiento de objetos en memoria.
//
// Existe porque la carga multimedia se verifica en la API —checksum, MIME real
// y antimalware— y eso es lógica propia, no del proveedor: exigir MinIO
// levantado para probarla dejaría el control sin pruebas en la práctica. Lo
// que el falso no simula es la firma SigV4; para eso está firma_test.go, que
// sí ejerce el cliente real.
//
// El navegador no existe aquí, así que las pruebas escriben los bytes con
// ponerObjeto y ponerParte, que es exactamente lo que haría un PUT contra la
// URL prefirmada que devuelve la API.
type almacenFalso struct {
	mu       sync.Mutex
	objetos  map[string]objetoFalso
	cargas   map[string]*cargaFalsa
	borrados []string
	abortos  []string
}

type objetoFalso struct {
	datos       []byte
	contentType string
}

type cargaFalsa struct {
	objectKey   string
	contentType string
	partes      map[int][]byte
}

func nuevoAlmacenFalso() *almacenFalso {
	return &almacenFalso{objetos: map[string]objetoFalso{}, cargas: map[string]*cargaFalsa{}}
}

// --- Lo que hace el navegador ---

// ponerObjeto simula el PUT del navegador contra la URL prefirmada simple.
func (a *almacenFalso) ponerObjeto(clave string, datos []byte, contentType string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.objetos[clave] = objetoFalso{datos: datos, contentType: contentType}
}

// ponerParte simula el PUT de una parte y devuelve su ETag, que es lo que el
// cliente tiene que reenviar al completar la carga.
func (a *almacenFalso) ponerParte(uploadID string, numero int, datos []byte) string {
	a.mu.Lock()
	defer a.mu.Unlock()
	c, ok := a.cargas[uploadID]
	if !ok {
		panic("almacenFalso: se subió una parte de una carga que no existe")
	}
	c.partes[numero] = datos
	return etagDe(numero, datos)
}

func etagDe(numero int, datos []byte) string {
	return fmt.Sprintf("\"parte-%d-%d\"", numero, len(datos))
}

func (a *almacenFalso) existe(clave string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	_, ok := a.objetos[clave]
	return ok
}

// --- AlmacenDeObjetos ---

func (a *almacenFalso) PresignedPutURL(_ context.Context, clave string, expiry time.Duration) (string, error) {
	return fmt.Sprintf("https://almacen.pruebas.local/%s?expira=%d", clave, int(expiry.Seconds())), nil
}

func (a *almacenFalso) InitiateMultipartUpload(_ context.Context, clave, contentType string) (string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	id := fmt.Sprintf("carga-%d", len(a.cargas)+1)
	a.cargas[id] = &cargaFalsa{objectKey: clave, contentType: contentType, partes: map[int][]byte{}}
	return id, nil
}

func (a *almacenFalso) PresignedUploadPartURL(_ context.Context, clave, uploadID string, numero int, expiry time.Duration) (string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if _, ok := a.cargas[uploadID]; !ok {
		return "", fmt.Errorf("almacenFalso: carga %s desconocida", uploadID)
	}
	return fmt.Sprintf("https://almacen.pruebas.local/%s?parte=%d&carga=%s&expira=%d",
		clave, numero, uploadID, int(expiry.Seconds())), nil
}

func (a *almacenFalso) CompleteMultipartUpload(_ context.Context, clave, uploadID string, partes []storage.ParteCargada) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	c, ok := a.cargas[uploadID]
	if !ok {
		return fmt.Errorf("almacenFalso: carga %s desconocida", uploadID)
	}
	ordenadas := append([]storage.ParteCargada(nil), partes...)
	sort.Slice(ordenadas, func(i, j int) bool { return ordenadas[i].Numero < ordenadas[j].Numero })

	var ensamblado []byte
	for _, p := range ordenadas {
		datos, ok := c.partes[p.Numero]
		if !ok {
			return fmt.Errorf("almacenFalso: falta la parte %d", p.Numero)
		}
		// El ETag es lo que delata una parte que no es la que se subió.
		if p.ETag != etagDe(p.Numero, datos) {
			return fmt.Errorf("almacenFalso: ETag de la parte %d no coincide", p.Numero)
		}
		ensamblado = append(ensamblado, datos...)
	}
	a.objetos[clave] = objetoFalso{datos: ensamblado, contentType: c.contentType}
	delete(a.cargas, uploadID)
	return nil
}

func (a *almacenFalso) AbortMultipartUpload(_ context.Context, _, uploadID string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.abortos = append(a.abortos, uploadID)
	delete(a.cargas, uploadID)
	return nil
}

func (a *almacenFalso) ListObjectParts(_ context.Context, _, uploadID string) ([]storage.ParteCargada, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	c, ok := a.cargas[uploadID]
	if !ok {
		return nil, fmt.Errorf("almacenFalso: carga %s desconocida", uploadID)
	}
	partes := make([]storage.ParteCargada, 0, len(c.partes))
	for numero, datos := range c.partes {
		partes = append(partes, storage.ParteCargada{
			Numero: numero, ETag: etagDe(numero, datos), Tamano: int64(len(datos)),
		})
	}
	sort.Slice(partes, func(i, j int) bool { return partes[i].Numero < partes[j].Numero })
	return partes, nil
}

func (a *almacenFalso) StatObject(_ context.Context, clave string) (storage.ObjetoInfo, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	o, ok := a.objetos[clave]
	if !ok {
		return storage.ObjetoInfo{}, fmt.Errorf("almacenFalso: no existe %s", clave)
	}
	tipo := o.contentType
	if tipo == "" {
		tipo = http.DetectContentType(o.datos)
	}
	return storage.ObjetoInfo{Tamano: int64(len(o.datos)), ContentType: tipo}, nil
}

func (a *almacenFalso) AbrirObjeto(_ context.Context, clave string) (io.ReadCloser, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	o, ok := a.objetos[clave]
	if !ok {
		return nil, fmt.Errorf("almacenFalso: no existe %s", clave)
	}
	return io.NopCloser(strings.NewReader(string(o.datos))), nil
}

// GuardarObjeto recibe lo que escribe la propia plataforma (la imagen de una
// insignia), no una carga de usuario.
func (a *almacenFalso) GuardarObjeto(_ context.Context, clave string, r io.Reader, _ int64, contentType string) error {
	datos, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	a.objetos[clave] = objetoFalso{datos: datos, contentType: contentType}
	return nil
}

// contenido devuelve los bytes de un objeto guardado, para que una prueba
// pueda mirar lo que la plataforma escribió.
func (a *almacenFalso) contenido(clave string) []byte {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.objetos[clave].datos
}

func (a *almacenFalso) RemoveObject(_ context.Context, clave string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	delete(a.objetos, clave)
	a.borrados = append(a.borrados, clave)
	return nil
}
