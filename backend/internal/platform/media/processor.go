// Package media implementa el procesamiento asíncrono de video y audio a
// HLS mediante FFmpeg (debe estar disponible en el PATH del contenedor del
// worker). El original siempre se conserva; nunca se hace upscaling por
// encima de la resolución fuente.
package media

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

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
}

// HandleProcessMedia es el asynq.HandlerFunc registrado para
// queue.TaskProcessMedia. Es idempotente: si el activo ya está en estado
// "ready", una entrega duplicada del mismo trabajo no reprocesa ni produce
// salidas repetidas.
func (p *Processor) HandleProcessMedia(ctx context.Context, t *asynq.Task) error {
	var payload queue.MediaProcessPayload
	if err := json.Unmarshal(t.Payload(), &payload); err != nil {
		return fmt.Errorf("media: payload inválido: %w", err)
	}

	asset, err := p.Assets.GetByID(ctx, payload.MediaAssetID)
	if err != nil {
		return fmt.Errorf("media: no se encontró el activo %s: %w", payload.MediaAssetID, err)
	}
	if asset.Status == "ready" {
		return nil // idempotencia: ya procesado, no repetir salida.
	}

	if err := p.Assets.MarkProcessing(ctx, asset.ID); err != nil {
		return err
	}

	hlsKey, err := p.transcode(ctx, payload)
	if err != nil {
		_ = p.Assets.MarkFailed(ctx, asset.ID, err.Error())
		_ = p.Courses.SetResourceProcessingStatus(ctx, payload.ResourceID, coursedomain.ProcessingFailed)
		return err // asynq reintentará con backoff hasta queue.MaxRetry, luego DLQ.
	}

	if err := p.Assets.MarkReady(ctx, asset.ID, hlsKey); err != nil {
		return err
	}
	return p.Courses.SetResourceProcessingStatus(ctx, payload.ResourceID, coursedomain.ProcessingReady)
}

func (p *Processor) transcode(ctx context.Context, payload queue.MediaProcessPayload) (hlsMasterKey string, err error) {
	workDir, err := os.MkdirTemp("", "mooc-media-*")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(workDir)

	srcPath := filepath.Join(workDir, "source")
	if err := p.Storage.DownloadToFile(ctx, payload.SourceObjectKey, srcPath); err != nil {
		return "", err
	}

	if payload.Kind == "audio" {
		return p.transcodeAudio(ctx, payload, workDir, srcPath)
	}
	return p.transcodeVideo(ctx, payload, workDir, srcPath)
}

func (p *Processor) transcodeVideo(ctx context.Context, payload queue.MediaProcessPayload, workDir, srcPath string) (string, error) {
	sourceHeight, err := probeHeight(ctx, srcPath)
	if err != nil {
		return "", fmt.Errorf("ffprobe: %w", err)
	}

	var chosen []rendition
	for _, r := range ladder {
		if r.Height <= sourceHeight {
			chosen = append(chosen, r)
		}
	}
	if len(chosen) == 0 {
		chosen = []rendition{{Height: sourceHeight, Bitrate: "800k"}}
	}

	var variants []string
	for _, r := range chosen {
		name := fmt.Sprintf("h%d", r.Height)
		playlist := filepath.Join(workDir, name+".m3u8")
		segmentPattern := filepath.Join(workDir, name+"_%03d.ts")

		cmd := exec.CommandContext(ctx, "ffmpeg", "-y", "-i", srcPath,
			"-vf", fmt.Sprintf("scale=-2:%d", r.Height),
			"-c:v", "h264", "-b:v", r.Bitrate, "-c:a", "aac",
			"-hls_time", "6", "-hls_playlist_type", "vod",
			"-hls_segment_filename", segmentPattern, playlist)
		if out, err := cmd.CombinedOutput(); err != nil {
			return "", fmt.Errorf("ffmpeg (%s): %w: %s", name, err, truncate(string(out), 500))
		}
		variants = append(variants, fmt.Sprintf("#EXT-X-STREAM-INF:BANDWIDTH=%s,RESOLUTION=x%d\n%s.m3u8", bitrateToBps(r.Bitrate), r.Height, name))
	}

	masterPath := filepath.Join(workDir, "master.m3u8")
	master := "#EXTM3U\n" + strings.Join(variants, "\n") + "\n"
	if err := os.WriteFile(masterPath, []byte(master), 0o644); err != nil {
		return "", err
	}

	return p.uploadDir(ctx, payload.ResourceID.String(), workDir)
}

func (p *Processor) transcodeAudio(ctx context.Context, payload queue.MediaProcessPayload, workDir, srcPath string) (string, error) {
	playlist := filepath.Join(workDir, "audio.m3u8")
	segmentPattern := filepath.Join(workDir, "audio_%03d.ts")

	cmd := exec.CommandContext(ctx, "ffmpeg", "-y", "-i", srcPath,
		"-c:a", "aac", "-b:a", "128k", "-vn",
		"-hls_time", "6", "-hls_playlist_type", "vod",
		"-hls_segment_filename", segmentPattern, playlist)
	if out, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("ffmpeg (audio): %w: %s", err, truncate(string(out), 500))
	}

	masterPath := filepath.Join(workDir, "master.m3u8")
	master := "#EXTM3U\n#EXT-X-STREAM-INF:BANDWIDTH=128000\naudio.m3u8\n"
	if err := os.WriteFile(masterPath, []byte(master), 0o644); err != nil {
		return "", err
	}

	return p.uploadDir(ctx, payload.ResourceID.String(), workDir)
}

func (p *Processor) uploadDir(ctx context.Context, resourceID, dir string) (masterKey string, err error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", err
	}
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
	}
	return fmt.Sprintf("hls/%s/master.m3u8", resourceID), nil
}

func probeHeight(ctx context.Context, path string) (int, error) {
	cmd := exec.CommandContext(ctx, "ffprobe", "-v", "error", "-select_streams", "v:0",
		"-show_entries", "stream=height", "-of", "csv=p=0", path)
	out, err := cmd.Output()
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(strings.TrimSpace(string(out)))
}

func bitrateToBps(b string) string {
	b = strings.TrimSuffix(b, "k")
	n, err := strconv.Atoi(b)
	if err != nil {
		return "1000000"
	}
	return strconv.Itoa(n * 1000)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
