// Package migrations incrusta las migraciones SQL en el binario.
//
// Viven aqui, junto a los .sql, para que exista una sola copia: el migrador de
// internal/platform/postgres las lee desde este sistema de archivos y la
// imagen de Docker no necesita montarlas ni traer una herramienta aparte.
package migrations

import "embed"

// Archivos contiene las migraciones. Convencion de nombres:
// NNNN_descripcion.up.sql y NNNN_descripcion.down.sql.
//
//go:embed *.sql
var Archivos embed.FS
