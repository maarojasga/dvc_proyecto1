// Package multimedia reconoce contenedores de audio y vídeo a partir de los
// primeros bytes, sin FFmpeg ni el nombre del archivo.
//
// La API no lleva FFmpeg: sondear con ffprobe al confirmar obligaría a
// descargar el original entero en la petición HTTP. Lo que sí se puede hacer
// aquí, igual que con las presentaciones, es rechazar lo que ni siquiera
// parece un contenedor reconocible. Un bloque de bytes aleatorios no debe
// encolarse: FFmpeg lo rechaza igual, pero después de tres reintentos y con
// el recurso en "failed" sin una explicación útil para quien lo subió.
package multimedia

import (
	"bytes"
	"errors"
)

// ErrContenedorNoReconocible indica un archivo que no empieza por ninguna
// firma de audio o vídeo conocida.
var ErrContenedorNoReconocible = errors.New("multimedia: el archivo no es un contenedor de audio o vídeo reconocible")

// Reconocible informa si la cabecera corresponde a un contenedor que el
// worker puede intentar transcodificar.
//
// Cubre lo que el olfateo de la biblioteca estándar ya clasifica como audio o
// vídeo, y además los contenedores que ese olfateo deja en
// application/octet-stream (mkv, buena parte de los mov, m4a). Lo que no
// cubre es un archivo corrupto con firma válida: eso lo corta ffprobe en el
// worker, sin reintentar.
func Reconocible(cabecera []byte) bool {
	if len(cabecera) < 4 {
		return false
	}
	switch {
	case len(cabecera) >= 8 && string(cabecera[4:8]) == "ftyp":
		// ISO BMFF: mp4, m4a, mov, 3gp.
		return true
	case bytes.HasPrefix(cabecera, []byte{0x1A, 0x45, 0xDF, 0xA3}):
		// EBML: mkv, webm.
		return true
	case bytes.HasPrefix(cabecera, []byte("OggS")):
		return true
	case bytes.HasPrefix(cabecera, []byte("fLaC")):
		return true
	case bytes.HasPrefix(cabecera, []byte("ID3")):
		return true
	case esMPEGAudio(cabecera):
		return true
	case esRIFF(cabecera):
		return true
	case esAIFF(cabecera):
		return true
	case bytes.HasPrefix(cabecera, []byte{0x30, 0x26, 0xB2, 0x75, 0x8E, 0x66, 0xCF, 0x11}):
		// ASF / WMA / WMV.
		return true
	case bytes.HasPrefix(cabecera, []byte{0x00, 0x00, 0x01, 0xBA}):
		// MPEG program stream.
		return true
	}
	return false
}

func esMPEGAudio(b []byte) bool {
	if len(b) < 2 || b[0] != 0xFF {
		return false
	}
	// Sincronización MPEG: 11 bits a 1. Cubre MP3 (0xFF 0xFB/F3/F2) y AAC
	// ADTS (0xFF 0xF1/F9).
	return b[1]&0xE0 == 0xE0
}

func esRIFF(b []byte) bool {
	if len(b) < 12 || !bytes.HasPrefix(b, []byte("RIFF")) {
		return false
	}
	tipo := string(b[8:12])
	return tipo == "WAVE" || tipo == "AVI "
}

func esAIFF(b []byte) bool {
	if len(b) < 12 || !bytes.HasPrefix(b, []byte("FORM")) {
		return false
	}
	tipo := string(b[8:12])
	return tipo == "AIFF" || tipo == "AIFC"
}
