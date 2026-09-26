package multimedia_test

import (
	"bytes"
	"testing"

	"github.com/DES-SOLUCIONES-CLOUD/proyecto-1/backend/internal/domain/multimedia"
)

func TestReconoceContenedoresHabituales(t *testing.T) {
	casos := map[string][]byte{
		"mp4":  append([]byte{0, 0, 0, 0x20}, []byte("ftypisom")...),
		"m4a":  append([]byte{0, 0, 0, 0x18}, []byte("ftypM4A ")...),
		"mkv":  {0x1A, 0x45, 0xDF, 0xA3, 0x01, 0x00, 0x00, 0x00},
		"ogg":  []byte("OggS\x00\x02"),
		"flac": []byte("fLaC\x00\x00\x00\x22"),
		"id3":  []byte("ID3\x04\x00\x00"),
		"mp3":  {0xFF, 0xFB, 0x90, 0x00},
		"aac":  {0xFF, 0xF1, 0x50, 0x80},
		"wav":  append([]byte("RIFF"), append([]byte{0, 0, 0, 0}, []byte("WAVE")...)...),
		"avi":  append([]byte("RIFF"), append([]byte{0, 0, 0, 0}, []byte("AVI ")...)...),
		"aiff": append([]byte("FORM"), append([]byte{0, 0, 0, 0}, []byte("AIFF")...)...),
	}
	for nombre, cabecera := range casos {
		if !multimedia.Reconocible(cabecera) {
			t.Errorf("%s debería reconocerse", nombre)
		}
	}
}

func TestRechazaLoQueNoEsAudioNiVideo(t *testing.T) {
	casos := map[string][]byte{
		"vacío":     {},
		"corto":     {0x00, 0x01},
		"basura":    bytes.Repeat([]byte{0x07}, 2048),
		"PDF":       []byte("%PDF-1.7\n"),
		"texto":     []byte("esto no es un audio"),
		"RIFF raro": append([]byte("RIFF"), append([]byte{0, 0, 0, 0}, []byte("CDXA")...)...),
	}
	for nombre, cabecera := range casos {
		if multimedia.Reconocible(cabecera) {
			t.Errorf("%s no debería reconocerse", nombre)
		}
	}
}
