// Package documentos convierte presentaciones (PPTX y ODP) a PDF de forma
// asíncrona, para que el visor de PDF de la plataforma pueda previsualizarlas.
//
// Reutiliza tal cual la maquinaria del procesamiento multimedia —toma
// exclusiva del trabajo, reintentos con backoff, dead-letter queue— porque el
// problema es el mismo: un trabajo lento que no puede bloquear la API y cuya
// doble entrega no debe producir salidas repetidas.
//
// La conversión en sí la hace LibreOffice en modo headless, que va instalado
// en la imagen del worker. Va detrás de una interfaz para que la lógica que la
// rodea —la que decide si procesar, qué guardar y cómo fallar— se pueda probar
// sin arrastrar LibreOffice a la suite.
package documentos

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/hibiken/asynq"

	coursedomain "github.com/DES-SOLUCIONES-CLOUD/proyecto-1/backend/internal/domain/course"
	"github.com/DES-SOLUCIONES-CLOUD/proyecto-1/backend/internal/domain/documento"
	"github.com/DES-SOLUCIONES-CLOUD/proyecto-1/backend/internal/platform/postgres"
	"github.com/DES-SOLUCIONES-CLOUD/proyecto-1/backend/internal/platform/queue"
)

// Almacen es lo que la conversión necesita del almacenamiento de objetos.
type Almacen interface {
	DownloadToFile(ctx context.Context, objectKey, destPath string) error
	UploadFile(ctx context.Context, objectKey, srcPath, contentType string) error
}

// Conversor produce un PDF a partir de un archivo de presentación.
//
// Recibe rutas locales y no claves de objeto: quien convierte no habla con el
// almacén, lo que permite sustituirlo en una prueba por algo que escriba un
// PDF de mentira.
type Conversor interface {
	APDF(ctx context.Context, origen, dirSalida string) (rutaPDF string, err error)
}

// LibreOffice es el conversor de producción.
type LibreOffice struct {
	// Binario es el ejecutable a invocar. Vacío usa "soffice".
	Binario string
	// Timeout acota una conversión. LibreOffice puede quedarse colgado con un
	// archivo malformado, y un worker bloqueado deja de consumir la cola.
	Timeout time.Duration
}

const timeoutPorDefecto = 5 * time.Minute

func (l LibreOffice) APDF(ctx context.Context, origen, dirSalida string) (string, error) {
	binario := l.Binario
	if binario == "" {
		binario = "soffice"
	}
	plazo := l.Timeout
	if plazo <= 0 {
		plazo = timeoutPorDefecto
	}
	ctx, cancel := context.WithTimeout(ctx, plazo)
	defer cancel()

	// -env:UserInstallation aísla el perfil por conversión. Sin esto, dos
	// conversiones simultáneas en el mismo worker comparten perfil y la
	// segunda falla o se queda esperando el bloqueo de la primera.
	perfil := filepath.Join(dirSalida, "perfil")
	cmd := exec.CommandContext(ctx, binario,
		"--headless", "--norestore",
		"-env:UserInstallation=file://"+perfil,
		"--convert-to", "pdf:impress_pdf_Export",
		"--outdir", dirSalida, origen)
	if salida, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("libreoffice: %w: %s", err, recortar(string(salida), 500))
	}

	// LibreOffice nombra la salida como el origen con extensión .pdf, así que
	// se deriva en lugar de buscar a ciegas en el directorio.
	base := strings.TrimSuffix(filepath.Base(origen), filepath.Ext(origen))
	ruta := filepath.Join(dirSalida, base+".pdf")
	if _, err := os.Stat(ruta); err != nil {
		return "", fmt.Errorf("libreoffice: no produjo el PDF esperado en %s: %w", ruta, err)
	}
	return ruta, nil
}

// Procesador consume los trabajos de conversión.
type Procesador struct {
	Storage   Almacen
	Assets    *postgres.MediaRepo
	Courses   *postgres.CourseRepo
	Conversor Conversor
	Log       *slog.Logger
}

func (p *Procesador) log() *slog.Logger {
	if p.Log != nil {
		return p.Log
	}
	return slog.Default()
}

func (p *Procesador) conversor() Conversor {
	if p.Conversor != nil {
		return p.Conversor
	}
	return LibreOffice{}
}

// HandleConvertDocument es el asynq.HandlerFunc de queue.TaskConvertDocument.
//
// Es idempotente por la misma vía que la transcodificación: reclamar el activo
// es un UPDATE condicional, así que una entrega duplicada se descarta en
// silencio en lugar de convertir dos veces.
func (p *Procesador) HandleConvertDocument(ctx context.Context, t *asynq.Task) error {
	var payload queue.DocumentConvertPayload
	if err := json.Unmarshal(t.Payload(), &payload); err != nil {
		return fmt.Errorf("documentos: payload inválido: %w", err)
	}

	activo, err := p.Assets.GetByID(ctx, payload.MediaAssetID)
	if err != nil {
		return fmt.Errorf("documentos: no se encontró el activo %s: %w", payload.MediaAssetID, err)
	}

	reclamado, err := p.Assets.Reclamar(ctx, activo.ID, time.Now().UTC())
	if err != nil {
		return err
	}
	if !reclamado {
		p.log().Info("documentos: entrega duplicada descartada",
			"activo", activo.ID, "recurso", payload.ResourceID, "estado", activo.Status)
		return nil
	}

	clavePDF, err := p.convertir(ctx, payload)
	if err != nil {
		_ = p.Assets.MarkFailed(ctx, activo.ID, err.Error())
		_ = p.Courses.SetResourceProcessingStatusInternal(ctx, payload.ResourceID, coursedomain.ProcessingFailed)
		return err // asynq reintenta con backoff y, agotados, archiva y alerta.
	}

	// El original se conserva: la conversión produce una vista previa, no
	// sustituye al archivo que subió el profesor.
	if err := p.Assets.MarcarPDFListo(ctx, activo.ID, clavePDF); err != nil {
		return err
	}
	return p.Courses.SetResourceProcessingStatusInternal(ctx, payload.ResourceID, coursedomain.ProcessingReady)
}

func (p *Procesador) convertir(ctx context.Context, payload queue.DocumentConvertPayload) (string, error) {
	dir, err := os.MkdirTemp("", "mooc-doc-*")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(dir)

	// La extensión importa: LibreOffice elige el filtro de entrada por ella, y
	// un archivo sin extensión se convierte mal o no se convierte.
	formato := documento.Formato(payload.Formato)
	extension := formato.Extension()
	if extension == "" {
		return "", documento.ErrFormatoNoSoportado
	}
	origen := filepath.Join(dir, "original"+extension)
	if err := p.Storage.DownloadToFile(ctx, payload.SourceObjectKey, origen); err != nil {
		return "", err
	}

	rutaPDF, err := p.conversor().APDF(ctx, origen, dir)
	if err != nil {
		return "", err
	}

	clave := ClaveDelPDF(payload.ResourceID.String())
	if err := p.Storage.UploadFile(ctx, clave, rutaPDF, "application/pdf"); err != nil {
		return "", err
	}
	return clave, nil
}

// ClaveDelPDF es dónde vive la vista previa de una presentación.
//
// Deriva del recurso y no del activo, de modo que volver a convertir
// sobrescribe la vista previa anterior en lugar de acumular copias huérfanas.
func ClaveDelPDF(resourceID string) string {
	return "presentaciones/" + resourceID + "/preview.pdf"
}

func recortar(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
