// Package cambios compara un borrador de actualización con la versión
// publicada y clasifica lo que ha cambiado.
//
// Sirve a dos lectores distintos. Al profesor, que antes de publicar necesita
// ver qué está a punto de cambiar sin repasar el árbol entero a ojo. Y a la
// migración del progreso, que necesita saber una cosa concreta: si el conjunto
// de recursos obligatorios visibles es el mismo. Mientras lo sea, lo que los
// estudiantes ya llevaban hecho sigue midiendo lo mismo; en cuanto cambia, el
// avance de todos pasa a calcularse sobre otra vara.
//
// La comparación se hace por `stable_id` y no por identificador de fila: esos
// identificadores son justamente los que un borrador de actualización conserva
// para que el progreso sobreviva a la publicación.
package cambios

import (
	"sort"

	"github.com/google/uuid"

	"github.com/DES-SOLUCIONES-CLOUD/proyecto-1/backend/internal/domain/course"
)

// TipoDeCambio es qué le pasó a un elemento entre una versión y la siguiente.
type TipoDeCambio string

const (
	Agregado   TipoDeCambio = "agregado"
	Eliminado  TipoDeCambio = "eliminado"
	Modificado TipoDeCambio = "modificado"
	Movido     TipoDeCambio = "movido"
)

// Elemento es el nivel de la jerarquía al que pertenece un cambio.
type Elemento string

const (
	DeModulo  Elemento = "modulo"
	DeUnidad  Elemento = "unidad"
	DeRecurso Elemento = "recurso"
)

// Alcance resume el impacto del borrador sobre el progreso ya registrado.
type Alcance string

const (
	// SinCambios: el borrador es idéntico a lo publicado.
	SinCambios Alcance = "ninguno"
	// Menor: cambia el contenido, los títulos o el orden, pero el conjunto de
	// recursos obligatorios visibles es el mismo. Lo que cada estudiante lleva
	// hecho sigue valiendo exactamente lo mismo.
	Menor Alcance = "menor"
	// Mayor: cambia lo que hay que completar para aprobar. El avance de todos
	// los inscritos se recalcula sobre otra base al publicar.
	Mayor Alcance = "mayor"
)

// Cambio describe una diferencia concreta.
type Cambio struct {
	Tipo     TipoDeCambio `json:"tipo"`
	Elemento Elemento     `json:"elemento"`
	StableID uuid.UUID    `json:"stable_id"`
	Titulo   string       `json:"titulo"`
	// Detalle explica el cambio en una línea, para que el profesor no tenga
	// que deducirlo comparando dos pantallas.
	Detalle string `json:"detalle,omitempty"`
	// AfectaProgreso marca los cambios que alteran el conjunto de obligatorios
	// visibles, que son los únicos que cambian lo que un estudiante debe hacer.
	AfectaProgreso bool `json:"afecta_progreso"`
}

// Clasificacion es el resultado de comparar dos versiones.
type Clasificacion struct {
	Alcance Alcance  `json:"alcance"`
	Cambios []Cambio `json:"cambios"`
	// ObligatoriosAgregados y ObligatoriosEliminados son los stable_id cuya
	// entrada o salida del conjunto de obligatorios cambia lo exigido.
	ObligatoriosAgregados  []uuid.UUID `json:"obligatorios_agregados,omitempty"`
	ObligatoriosEliminados []uuid.UUID `json:"obligatorios_eliminados,omitempty"`
}

// nodo aplana un elemento del árbol para poder compararlo por stable_id.
type nodo struct {
	elemento  Elemento
	titulo    string
	posicion  int
	padre     uuid.UUID
	tipo      string
	visible   bool
	requerido bool
	// huella resume lo que, si cambia, hace al elemento "modificado".
	huella string
}

// Comparar clasifica lo que el borrador cambia respecto a lo publicado.
//
// Recibe los dos árboles ya cargados. Si `publicado` viene vacío —un curso que
// nunca se publicó— todo es nuevo, pero el alcance es "ninguno": no hay
// progreso registrado al que afectar.
func Comparar(publicado, borrador []course.Module) Clasificacion {
	antes := aplanar(publicado)
	despues := aplanar(borrador)

	var cambios []Cambio
	for id, nuevo := range despues {
		viejo, existia := antes[id]
		switch {
		case !existia:
			cambios = append(cambios, Cambio{
				Tipo: Agregado, Elemento: nuevo.elemento, StableID: id, Titulo: nuevo.titulo,
				Detalle:        descripcionDeAlta(nuevo),
				AfectaProgreso: cuentaParaElProgreso(nuevo),
			})
		case viejo.huella != nuevo.huella:
			cambios = append(cambios, Cambio{
				Tipo: Modificado, Elemento: nuevo.elemento, StableID: id, Titulo: nuevo.titulo,
				Detalle:        descripcionDeCambio(viejo, nuevo),
				AfectaProgreso: cuentaParaElProgreso(viejo) != cuentaParaElProgreso(nuevo),
			})
		case viejo.posicion != nuevo.posicion || viejo.padre != nuevo.padre:
			// Mover no altera lo que hay que hacer, solo en qué orden se
			// encuentra: el progreso se sigue midiendo por stable_id.
			cambios = append(cambios, Cambio{
				Tipo: Movido, Elemento: nuevo.elemento, StableID: id, Titulo: nuevo.titulo,
				Detalle: "cambió de lugar", AfectaProgreso: false,
			})
		}
	}
	for id, viejo := range antes {
		if _, sigue := despues[id]; !sigue {
			cambios = append(cambios, Cambio{
				Tipo: Eliminado, Elemento: viejo.elemento, StableID: id, Titulo: viejo.titulo,
				Detalle:        "ya no forma parte del curso",
				AfectaProgreso: cuentaParaElProgreso(viejo),
			})
		}
	}

	// Orden estable: primero por nivel, luego por título. Sin esto la lista
	// saldría en el orden aleatorio de recorrer un mapa y no se podría
	// comparar una respuesta con la siguiente.
	sort.SliceStable(cambios, func(i, j int) bool {
		if cambios[i].Elemento != cambios[j].Elemento {
			return orden(cambios[i].Elemento) < orden(cambios[j].Elemento)
		}
		if cambios[i].Tipo != cambios[j].Tipo {
			return cambios[i].Tipo < cambios[j].Tipo
		}
		return cambios[i].Titulo < cambios[j].Titulo
	})

	agregados := diferencia(obligatorios(despues), obligatorios(antes))
	eliminados := diferencia(obligatorios(antes), obligatorios(despues))

	return Clasificacion{
		Alcance:                alcanceDe(len(cambios), len(agregados)+len(eliminados), len(antes)),
		Cambios:                cambios,
		ObligatoriosAgregados:  agregados,
		ObligatoriosEliminados: eliminados,
	}
}

// alcanceDe decide el impacto.
//
// Un curso que nunca se publicó (sin elementos "antes") no tiene inscritos ni
// progreso al que afectar, así que su borrador no es una actualización mayor
// por mucho contenido que traiga.
func alcanceDe(totalCambios, cambiosDeObligatorios, elementosPrevios int) Alcance {
	switch {
	case totalCambios == 0:
		return SinCambios
	case elementosPrevios == 0:
		return Menor
	case cambiosDeObligatorios > 0:
		return Mayor
	default:
		return Menor
	}
}

// cuentaParaElProgreso indica si el elemento entra en el cálculo del avance.
//
// Solo los recursos visibles y obligatorios: un módulo o una unidad no se
// completan por sí mismos, y un recurso opcional u oculto no exige nada.
func cuentaParaElProgreso(n nodo) bool {
	return n.elemento == DeRecurso && n.visible && n.requerido
}

func obligatorios(nodos map[uuid.UUID]nodo) map[uuid.UUID]struct{} {
	out := map[uuid.UUID]struct{}{}
	for id, n := range nodos {
		if cuentaParaElProgreso(n) {
			out[id] = struct{}{}
		}
	}
	return out
}

func diferencia(a, b map[uuid.UUID]struct{}) []uuid.UUID {
	var out []uuid.UUID
	for id := range a {
		if _, hay := b[id]; !hay {
			out = append(out, id)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].String() < out[j].String() })
	return out
}

func orden(e Elemento) int {
	switch e {
	case DeModulo:
		return 0
	case DeUnidad:
		return 1
	default:
		return 2
	}
}

// aplanar recorre el árbol y lo indexa por stable_id.
func aplanar(modulos []course.Module) map[uuid.UUID]nodo {
	out := map[uuid.UUID]nodo{}
	for _, m := range modulos {
		out[m.StableID] = nodo{
			elemento: DeModulo, titulo: m.Title, posicion: m.Position,
			huella: m.Title,
		}
		for _, u := range m.Units {
			out[u.StableID] = nodo{
				elemento: DeUnidad, titulo: u.Title, posicion: u.Position,
				padre:  m.StableID,
				huella: u.Title,
			}
			for _, r := range u.Resources {
				out[r.StableID] = nodo{
					elemento: DeRecurso, titulo: r.Title, posicion: r.Position,
					padre: u.StableID, tipo: string(r.Type),
					visible: r.Visible, requerido: r.Required,
					huella: huellaDeRecurso(r),
				}
			}
		}
	}
	return out
}

// huellaDeRecurso resume lo que hace distinto a un recurso.
//
// Incluye el contenido porque reescribir una lectura es un cambio que el
// profesor quiere ver listado; no incluye la posición, que se clasifica aparte
// como movimiento para no confundir reordenar con reescribir.
func huellaDeRecurso(r course.Resource) string {
	return r.Title + "\x00" + string(r.Type) +
		"\x00" + boolACadena(r.Visible) + boolACadena(r.Required) + boolACadena(r.Downloadable) +
		"\x00" + r.TextContentMD + "\x00" + r.ExternalURL + "\x00" + r.ObjectKey
}

func boolACadena(b bool) string {
	if b {
		return "1"
	}
	return "0"
}

// descripcionDeAlta explica qué supone un elemento nuevo.
func descripcionDeAlta(n nodo) string {
	if cuentaParaElProgreso(n) {
		return "nuevo recurso obligatorio: los estudiantes tendrán que completarlo"
	}
	if n.elemento == DeRecurso {
		return "nuevo recurso, no obligatorio"
	}
	return "nuevo"
}

// descripcionDeCambio nombra lo que cambió, priorizando lo que altera el
// progreso sobre lo que solo altera la presentación.
func descripcionDeCambio(viejo, nuevo nodo) string {
	switch {
	case viejo.requerido && !nuevo.requerido:
		return "deja de ser obligatorio"
	case !viejo.requerido && nuevo.requerido:
		return "pasa a ser obligatorio"
	case viejo.visible && !nuevo.visible:
		return "deja de estar visible"
	case !viejo.visible && nuevo.visible:
		return "pasa a estar visible"
	case viejo.tipo != nuevo.tipo:
		return "cambia de tipo: " + viejo.tipo + " → " + nuevo.tipo
	case viejo.titulo != nuevo.titulo:
		return "cambia el título: " + viejo.titulo + " → " + nuevo.titulo
	default:
		return "cambia el contenido"
	}
}
