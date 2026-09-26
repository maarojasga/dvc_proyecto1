// Package media implementa el procesamiento asíncrono de video y audio a
// HLS mediante FFmpeg (debe estar disponible en el PATH del contenedor del
// worker). El original siempre se conserva; nunca se hace upscaling por
// encima de la resolución fuente.
package media

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/hibiken/asynq"

	coursedomain "github.com/DES-SOLUCIONES-CLOUD/proyecto-1/backend/internal/domain/course"
	"github.com/DES-SOLUCIONES-CLOUD/proyecto-1/backend/internal/platform/postgres"
	"github.com/DES-SOLUCIONES-CLOUD/proyecto-1/backend/internal/platform/queue"
	"github.com/DES-SOLUCIONES-CLOUD/proyecto-1/backend/internal/platform/storage"
)

type rendition struct {
	Height  int
	Bitrate string
}

// ladder es la escalera de calidades candidata, de mayor a menor; se filtra
// por la altura real del original para nunca hacer upscaling.
var ladder = []rendition{
	{1080, "5000k"},
	{720, "2800k"},
	{480, "1400k"},
	{360, "800k"},
}

type Processor struct {
	Storage *storage.Client
	Assets  *postgres.MediaRepo
	Courses *postgres.CourseRepo
	Log     *slog.Logger
}

func (p *Processor) log() *slog.Logger {
	if p.Log != nil {
		return p.Log
	}
	return slog.Default()
}

// HandleProcessMedia es el asynq.HandlerFunc registrado para
// queue.TaskProcessMedia. Es idempotente: si el activo ya está en estado
// "ready", una entrega duplicada del mismo trabajo no reprocesa ni produce
// salidas repetidas.
func (p *Processor) HandleProcessMedia(ctx context.Context, t *asynq.Task) error {
	inicio := time.Now()

	var payload queue.MediaProcessPayload
	if err := json.Unmarshal(t.Payload(), &payload); err != nil {
		return fmt.Errorf("media: payload inválido: %w", err)
	}

	asset, err := p.Assets.GetByID(ctx, payload.MediaAssetID)
	if err != nil {
		return fmt.Errorf("media: no se encontró el activo %s: %w", payload.MediaAssetID, err)
	}

	// Tomar el trabajo es un UPDATE condicional: si el activo ya está listo o
	// si otro worker lo está procesando con el arrendamiento vigente, este
	// intento se descarta en silencio. Es lo que hace que una doble entrega no
	// produzca salidas repetidas, incluso si llega mientras el primer intento
	// sigue corriendo.
	reclamado, err := p.Assets.Reclamar(ctx, asset.ID, time.Now().UTC())
	if err != nil {
		return err
	}
	if !reclamado {
		p.log().Info("media: entrega duplicada descartada",
			"activo", asset.ID, "recurso", payload.ResourceID, "estado", asset.Status)
		return nil
	}

	// El recurso pudo borrarse mientras el trabajo esperaba en la cola.
	// Transcodificar ahora sería CPU quemada para un resultado que ningún
	// endpoint va a leer nunca, así que se abandona antes del trabajo
	// pesado en lugar de después.
	existe, err := p.Courses.ResourceExists(ctx, payload.ResourceID)
	if err != nil {
		return err
	}
	if !existe {
		p.log().Info("media: recurso eliminado, se descarta el trabajo",
			"activo", asset.ID, "recurso", payload.ResourceID)
		return nil
	}

	// A partir de aquí el trabajo es de este worker y empieza lo caro. Los
	// registros del camino feliz cuelgan de este logger para que todas las
	// líneas de un mismo trabajo compartan activo, recurso e intento.
	log := p.log().With(append(
		[]any{"activo", asset.ID, "recurso", payload.ResourceID, "tipo", payload.Kind},
		queue.AtributosDeLaTarea(ctx)...,
	)...)
	log.Info("media: trabajo aceptado, empieza la transcodificación", "origen", payload.SourceObjectKey)

	hlsKey, err := p.transcode(ctx, log, payload)
	if err != nil {
		_ = p.Assets.MarkFailed(ctx, asset.ID, err.Error())
		_ = p.Courses.SetResourceProcessingStatusInternal(ctx, payload.ResourceID, coursedomain.ProcessingFailed)
		if errors.Is(err, ErrEntradaInvalida) {
			// Reintentar el mismo original ilegible no lo vuelve reproducible.
			return fmt.Errorf("%w: %w", err, asynq.SkipRetry)
		}
		return err // asynq reintentará con backoff hasta queue.MaxRetry, luego DLQ.
	}

	if err := p.Assets.MarkReady(ctx, asset.ID, hlsKey); err != nil {
		return err
	}
	if err := p.Courses.SetResourceProcessingStatusInternal(ctx, payload.ResourceID, coursedomain.ProcessingReady); err != nil {
		return err
	}

	// El cierre va después de escribir el estado, no antes: lo que acredita
	// que el recurso quedó listo es la transición en la base, y anunciarla
	// mientras todavía puede fallar daría un registro que miente.
	log.Info("media: activo listo",
		"hls", hlsKey, "estado", coursedomain.ProcessingReady,
		"duracion_ms", time.Since(inicio).Milliseconds())
	return nil
}

func (p *Processor) transcode(ctx context.Context, log *slog.Logger, payload queue.MediaProcessPayload) (hlsMasterKey string, err error) {
	workDir, err := os.MkdirTemp("", "mooc-media-*")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(workDir)

	descarga := time.Now()
	srcPath := filepath.Join(workDir, "source")
	if err := p.Storage.DownloadToFile(ctx, payload.SourceObjectKey, srcPath); err != nil {
		return "", err
	}
	log.Info("media: original descargado", "bytes", tamano(srcPath),
		"duracion_ms", time.Since(descarga).Milliseconds())

	if payload.Kind == "audio" {
		return p.transcodeAudio(ctx, log, payload, workDir, srcPath)
	}
	return p.transcodeVideo(ctx, log, payload, workDir, srcPath)
}

func (p *Processor) transcodeVideo(ctx context.Context, log *slog.Logger, payload queue.MediaProcessPayload, workDir, srcPath string) (string, error) {
	ancho, alto, err := probeDimensiones(ctx, srcPath)
	if err != nil {
		return "", err
	}

	calidades := seleccionarCalidades(alto)
	// Que no haya upscaling es una decisión que no deja rastro en la salida:
	// un HLS de tres calidades no dice si se descartaron dos o si nunca se
	// consideraron. Registrar la escalera junto a la resolución del original
	// lo hace comprobable sin abrir la base ni el almacén.
	log.Info("media: escalera elegida sin upscaling",
		"original", fmt.Sprintf("%dx%d", ancho, alto),
		"calidades", nombresDeCalidades(calidades),
		"descartadas", len(ladder)-len(calidades))

	for _, r := range calidades {
		name := nombreCalidad(r)
		playlist := filepath.Join(workDir, name+".m3u8")
		segmentPattern := filepath.Join(workDir, name+"_%03d.ts")

		empezo := time.Now()
		cmd := exec.CommandContext(ctx, "ffmpeg", "-y", "-i", srcPath,
			"-vf", fmt.Sprintf("scale=-2:%d", r.Height),
			"-c:v", "h264", "-b:v", r.Bitrate, "-c:a", "aac",
			"-hls_time", "6", "-hls_playlist_type", "vod",
			"-hls_segment_filename", segmentPattern, playlist)
		if out, err := cmd.CombinedOutput(); err != nil {
			return "", errorDeFFmpeg(name, err, out)
		}
		log.Info("media: calidad transcodificada",
			"calidad", name, "bitrate", r.Bitrate,
			"duracion_ms", time.Since(empezo).Milliseconds())
	}

	masterPath := filepath.Join(workDir, "master.m3u8")
	if err := os.WriteFile(masterPath, []byte(masterDeVideo(calidades, ancho, alto)), 0o600); err != nil {
		return "", err
	}

	return p.uploadDir(ctx, log, payload.ResourceID.String(), workDir)
}

// seleccionarCalidades descarta las calidades por encima del original: subir
// de resolución no añade detalle y multiplica el costo de transcodificación y
// de entrega. Si el original es más bajo que toda la escalera, se emite una
// sola variante a su resolución nativa.
func seleccionarCalidades(alturaOriginal int) []rendition {
	var elegidas []rendition
	for _, r := range ladder {
		if r.Height <= alturaOriginal {
			elegidas = append(elegidas, r)
		}
	}
	if len(elegidas) == 0 {
		return []rendition{{Height: alturaOriginal, Bitrate: "800k"}}
	}
	return elegidas
}

func nombreCalidad(r rendition) string { return fmt.Sprintf("h%d", r.Height) }

func nombresDeCalidades(calidades []rendition) []string {
	nombres := make([]string, 0, len(calidades))
	for _, r := range calidades {
		nombres = append(nombres, nombreCalidad(r))
	}
	return nombres
}

// anchoEscalado reproduce lo que hace scale=-2:alto en FFmpeg: conserva la
// relación de aspecto y redondea a un número par, que es lo que exige el
// submuestreo de croma 4:2:0.
func anchoEscalado(anchoOriginal, alturaOriginal, altura int) int {
	if alturaOriginal <= 0 {
		return 0
	}
	ancho := int(math.Round(float64(anchoOriginal) * float64(altura) / float64(alturaOriginal)))
	if ancho%2 != 0 {
		ancho++
	}
	return ancho
}

// masterDeVideo arma la lista maestra.
//
// RESOLUTION debe ir como ANCHOxALTO: antes se emitía sin el ancho ("x1080"),
// que no es un valor válido y deja al reproductor sin saber a qué variante
// cambiar.
func masterDeVideo(calidades []rendition, anchoOriginal, alturaOriginal int) string {
	var b strings.Builder
	b.WriteString("#EXTM3U\n#EXT-X-VERSION:3\n")
	for _, r := range calidades {
		ancho := anchoEscalado(anchoOriginal, alturaOriginal, r.Height)
		fmt.Fprintf(&b, "#EXT-X-STREAM-INF:BANDWIDTH=%s,RESOLUTION=%dx%d\n%s.m3u8\n",
			bitrateToBps(r.Bitrate), ancho, r.Height, nombreCalidad(r))
	}
	return b.String()
}

func (p *Processor) transcodeAudio(ctx context.Context, log *slog.Logger, payload queue.MediaProcessPayload, workDir, srcPath string) (string, error) {
	if err := probePista(ctx, srcPath, "a:0"); err != nil {
		return "", err
	}

	empezo := time.Now()
	playlist := filepath.Join(workDir, "audio.m3u8")
	segmentPattern := filepath.Join(workDir, "audio_%03d.ts")

	cmd := exec.CommandContext(ctx, "ffmpeg", "-y", "-i", srcPath,
		"-c:a", "aac", "-b:a", "128k", "-vn",
		"-hls_time", "6", "-hls_playlist_type", "vod",
		"-hls_segment_filename", segmentPattern, playlist)
	if out, err := cmd.CombinedOutput(); err != nil {
		return "", errorDeFFmpeg("audio", err, out)
	}
	log.Info("media: pista de audio transcodificada",
		"bitrate", "128k", "duracion_ms", time.Since(empezo).Milliseconds())

	masterPath := filepath.Join(workDir, "master.m3u8")
	if err := os.WriteFile(masterPath, []byte(masterDeAudio()), 0o600); err != nil {
		return "", err
	}

	return p.uploadDir(ctx, log, payload.ResourceID.String(), workDir)
}

func (p *Processor) uploadDir(ctx context.Context, log *slog.Logger, resourceID, dir string) (masterKey string, err error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", err
	}
	empezo := time.Now()
	var archivos int
	var bytes int64
	for _, e := range entries {
		if e.IsDir() || e.Name() == "source" {
			continue
		}
		localPath := filepath.Join(dir, e.Name())
		objectKey := fmt.Sprintf("hls/%s/%s", resourceID, e.Name())
		contentType := "application/octet-stream"
		if strings.HasSuffix(e.Name(), ".m3u8") {
			contentType = "application/vnd.apple.mpegurl"
		} else if strings.HasSuffix(e.Name(), ".ts") {
			contentType = "video/mp2t"
		}
		if err := p.Storage.UploadFile(ctx, objectKey, localPath, contentType); err != nil {
			return "", err
		}
		archivos++
		bytes += tamano(localPath)
	}
	log.Info("media: HLS publicado en el almacén",
		"archivos", archivos, "bytes", bytes,
		"prefijo", fmt.Sprintf("hls/%s/", resourceID),
		"duracion_ms", time.Since(empezo).Milliseconds())
	return fmt.Sprintf("hls/%s/master.m3u8", resourceID), nil
}

// tamano es el peso de un archivo, o 0 si no se puede leer. Solo alimenta
// registros, así que un fallo aquí no debe tumbar un trabajo que va bien.
func tamano(path string) int64 {
	info, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return info.Size()
}

// masterDeAudio arma la lista maestra de una pista sin video.
func masterDeAudio() string {
	return "#EXTM3U\n#EXT-X-VERSION:3\n#EXT-X-STREAM-INF:BANDWIDTH=128000,CODECS=\"mp4a.40.2\"\naudio.m3u8\n"
}

// ErrEntradaInvalida indica un original que FFmpeg no puede abrir. No es un
// fallo transitorio: reintentarlo con backoff solo reproduce el mismo error.
var ErrEntradaInvalida = errors.New("el original no contiene una pista reproducible")

// probePista comprueba que ffprobe vea al menos un flujo del tipo pedido.
func probePista(ctx context.Context, path, selector string) error {
	cmd := exec.CommandContext(ctx, "ffprobe", "-v", "error", "-select_streams", selector,
		"-show_entries", "stream=codec_type", "-of", "csv=p=0", path)
	out, err := cmd.CombinedOutput()
	if ctx.Err() != nil {
		return ctx.Err()
	}
	pista := strings.TrimSpace(string(out))
	if err != nil || pista == "" {
		if entradaIrrecuperable(err, out) || pista == "" {
			return fmt.Errorf("%w: %s", ErrEntradaInvalida, colaDe(string(out), 400))
		}
		return fmt.Errorf("ffprobe: %w: %s", err, colaDe(string(out), 400))
	}
	return nil
}

// probeDimensiones lee ancho y alto del primer flujo de video.
func probeDimensiones(ctx context.Context, path string) (ancho, alto int, err error) {
	cmd := exec.CommandContext(ctx, "ffprobe", "-v", "error", "-select_streams", "v:0",
		"-show_entries", "stream=width,height", "-of", "csv=p=0:s=x", path)
	out, err := cmd.CombinedOutput()
	if ctx.Err() != nil {
		return 0, 0, ctx.Err()
	}
	if err != nil {
		if entradaIrrecuperable(err, out) {
			return 0, 0, fmt.Errorf("%w: %s", ErrEntradaInvalida, colaDe(string(out), 400))
		}
		return 0, 0, fmt.Errorf("ffprobe: %w: %s", err, colaDe(string(out), 400))
	}
	return parsearDimensiones(string(out))
}

// parsearDimensiones interpreta la salida "ANCHOxALTO" de ffprobe.
func parsearDimensiones(salida string) (ancho, alto int, err error) {
	campos := strings.Split(strings.TrimSpace(salida), "x")
	if len(campos) != 2 {
		return 0, 0, fmt.Errorf("media: dimensiones ilegibles: %q", strings.TrimSpace(salida))
	}
	if ancho, err = strconv.Atoi(campos[0]); err != nil {
		return 0, 0, fmt.Errorf("media: ancho ilegible: %w", err)
	}
	if alto, err = strconv.Atoi(campos[1]); err != nil {
		return 0, 0, fmt.Errorf("media: alto ilegible: %w", err)
	}
	return ancho, alto, nil
}

func errorDeFFmpeg(nombre string, err error, out []byte) error {
	wrapped := fmt.Errorf("ffmpeg (%s): %w: %s", nombre, err, colaDe(string(out), 500))
	if entradaIrrecuperable(err, out) {
		return fmt.Errorf("%w: %v", ErrEntradaInvalida, wrapped)
	}
	return wrapped
}

func entradaIrrecuperable(err error, out []byte) bool {
	var exit *exec.ExitError
	if errors.As(err, &exit) && exit.ExitCode() == 183 {
		return true
	}
	s := string(out)
	return strings.Contains(s, "Invalid data found") ||
		strings.Contains(s, "Error opening input") ||
		strings.Contains(s, "does not contain any stream")
}

func bitrateToBps(b string) string {
	b = strings.TrimSuffix(b, "k")
	n, err := strconv.Atoi(b)
	if err != nil {
		return "1000000"
	}
	return strconv.Itoa(n * 1000)
}

// colaDe conserva el final de una salida larga. FFmpeg escribe el banner al
// principio y el motivo del fallo al final: recortar por el inicio dejaba
// solo la versión y escondía "Invalid data found when processing input".
func colaDe(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return "..." + s[len(s)-n:]
}
