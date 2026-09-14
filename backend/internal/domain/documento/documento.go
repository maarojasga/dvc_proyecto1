// Package documento reconoce los formatos de presentación que la plataforma
// convierte a PDF.
//
// El reconocimiento se hace sobre los bytes y no sobre el nombre ni el
// Content-Type declarado, por la misma razón que en el resto de la ingesta:
// los dos los escribe quien sube el archivo.
//
// PPTX y ODP son contenedores ZIP, así que ambos empiezan por "PK\x03\x04" y
// el olfateo genérico de la biblioteca estándar los da por application/zip.
// Distinguirlos exige mirar un poco más adentro: el nombre de la primera
// entrada del contenedor, que en los dos formatos está fijado por su
// especificación y aparece en la cabecera local, sin necesidad de
// descomprimir nada.
package documento

import (
	"encoding/binary"
	"errors"
	"strings"
)

// Formato es un formato de presentación reconocido.
type Formato string

const (
	// PPTX es OOXML (PowerPoint 2007 en adelante).
	PPTX Formato = "pptx"
	// ODP es OpenDocument Presentation.
	ODP Formato = "odp"
	// Desconocido es todo lo demás.
	Desconocido Formato = ""
)

// ErrFormatoNoSoportado indica un archivo que no es una presentación que se
// sepa convertir.
var ErrFormatoNoSoportado = errors.New("documento: el archivo no es una presentación PPTX ni ODP")

// firmaZIP es la cabecera de un archivo local dentro de un contenedor ZIP.
var firmaZIP = []byte{'P', 'K', 0x03, 0x04}

// Un ZIP arranca con una cabecera local de 30 bytes y, a continuación, el
// nombre de la primera entrada. Ahí es donde los dos formatos se delatan.
const (
	offsetLargoDelNombre = 26
	offsetDelNombre      = 30
)

// tipoODF es el contenido del archivo "mimetype" de una presentación
// OpenDocument. La especificación exige que sea la primera entrada del
// contenedor y que se almacene sin comprimir, justamente para que un programa
// pueda reconocer el formato leyendo unos pocos bytes.
const tipoODF = "application/vnd.oasis.opendocument.presentation"

// Detectar reconoce el formato a partir del principio del archivo.
//
// Le bastan unos cientos de bytes: no hace falta —ni sería sensato— cargar en
// memoria una presentación entera para saber qué es.
func Detectar(cabecera []byte) Formato {
	if len(cabecera) < offsetDelNombre || !tienePrefijo(cabecera, firmaZIP) {
		return Desconocido
	}

	largo := int(binary.LittleEndian.Uint16(cabecera[offsetLargoDelNombre : offsetLargoDelNombre+2]))
	fin := offsetDelNombre + largo
	if largo <= 0 || fin > len(cabecera) {
		return Desconocido
	}
	nombre := string(cabecera[offsetDelNombre:fin])

	switch {
	case nombre == "mimetype":
		// En ODF el contenido del "mimetype" va justo detrás del nombre, sin
		// comprimir, y distingue una presentación de un texto o una hoja de
		// cálculo, que comparten contenedor.
		resto := cabecera[fin:]
		if strings.HasPrefix(string(resto), tipoODF) {
			return ODP
		}
		return Desconocido
	case nombre == "[Content_Types].xml":
		// Todo OOXML lo lleva como primera entrada. Que sea concretamente una
		// presentación y no un documento de Word lo dirá la conversión, que
		// falla de forma limpia y deja el recurso en "failed".
		return PPTX
	case strings.HasPrefix(nombre, "ppt/"):
		// Algunos generadores ordenan las entradas de otra manera.
		return PPTX
	}
	return Desconocido
}

// BytesNecesarios es cuánto hay que leer del principio del archivo para que
// Detectar pueda decidir. Sobra para el nombre de la primera entrada y, en
// ODF, para el tipo que va detrás.
const BytesNecesarios = 512

func tienePrefijo(b, prefijo []byte) bool {
	if len(b) < len(prefijo) {
		return false
	}
	for i := range prefijo {
		if b[i] != prefijo[i] {
			return false
		}
	}
	return true
}

// Extension es el sufijo con que hay que guardar el original para que el
// convertidor lo reconozca. LibreOffice decide el filtro de entrada por la
// extensión, así que un archivo sin ella se convierte mal o no se convierte.
func (f Formato) Extension() string {
	switch f {
	case PPTX:
		return ".pptx"
	case ODP:
		return ".odp"
	}
	return ""
}
