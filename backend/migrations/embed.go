// Package migrations embebe los archivos SQL de este directorio para que el
// binario de la API los pueda aplicar en el arranque sin depender de
// herramientas externas.
package migrations

import "embed"

//go:embed *.sql
var FS embed.FS
