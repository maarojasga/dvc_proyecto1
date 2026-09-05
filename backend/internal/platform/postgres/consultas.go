package postgres

import "strings"

// formatearConsulta sustituye el marcador %s de una plantilla de consulta por
// la condicion indicada. Las condiciones son literales del propio paquete,
// nunca entrada del usuario: los valores siempre viajan como parametros.
func formatearConsulta(plantilla, condicion string) string {
	return strings.Replace(plantilla, "%s", condicion, 1)
}
