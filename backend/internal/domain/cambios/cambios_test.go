package cambios_test

import (
	"testing"

	"github.com/google/uuid"

	"github.com/DES-SOLUCIONES-CLOUD/proyecto-1/backend/internal/domain/cambios"
	"github.com/DES-SOLUCIONES-CLOUD/proyecto-1/backend/internal/domain/course"
)

// Identificadores fijos: lo que se compara es el stable_id, así que las
// pruebas los conservan entre las dos versiones igual que lo hace un borrador
// de actualización.
var (
	modulo  = uuid.MustParse("11111111-1111-1111-1111-111111111111")
	unidad  = uuid.MustParse("22222222-2222-2222-2222-222222222222")
	lectura = uuid.MustParse("33333333-3333-3333-3333-333333333333")
	video   = uuid.MustParse("44444444-4444-4444-4444-444444444444")
)

// arbol arma un curso de un módulo y una unidad con los recursos dados.
func arbol(recursos ...course.Resource) []course.Module {
	return []course.Module{{
		StableID: modulo, Title: "Módulo 1", Position: 1,
		Units: []course.Unit{{
			StableID: unidad, Title: "Unidad 1", Position: 1,
			Resources: recursos,
		}},
	}}
}

func recurso(id uuid.UUID, titulo string, visible, requerido bool) course.Resource {
	return course.Resource{
		StableID: id, Title: titulo, Type: course.ResourceText, Position: 1,
		Visible: visible, Required: requerido, TextContentMD: "contenido",
	}
}

func buscar(c cambios.Clasificacion, id uuid.UUID) (cambios.Cambio, bool) {
	for _, x := range c.Cambios {
		if x.StableID == id {
			return x, true
		}
	}
	return cambios.Cambio{}, false
}

func TestUnBorradorIdenticoNoTieneCambios(t *testing.T) {
	a := arbol(recurso(lectura, "Lectura", true, true))
	c := cambios.Comparar(a, arbol(recurso(lectura, "Lectura", true, true)))

	if c.Alcance != cambios.SinCambios {
		t.Errorf("alcance %q, se esperaba ninguno", c.Alcance)
	}
	if len(c.Cambios) != 0 {
		t.Errorf("se listaron cambios sobre un borrador idéntico: %+v", c.Cambios)
	}
	_ = a
}

// Reescribir una lectura es un cambio que el profesor quiere ver, pero no
// altera lo que hay que completar.
func TestReescribirElContenidoEsUnCambioMenor(t *testing.T) {
	antes := arbol(recurso(lectura, "Lectura", true, true))
	despues := arbol(course.Resource{
		StableID: lectura, Title: "Lectura", Type: course.ResourceText, Position: 1,
		Visible: true, Required: true, TextContentMD: "contenido reescrito",
	})

	c := cambios.Comparar(antes, despues)
	if c.Alcance != cambios.Menor {
		t.Fatalf("alcance %q, se esperaba menor", c.Alcance)
	}
	cambio, ok := buscar(c, lectura)
	if !ok {
		t.Fatal("no se listó el cambio del recurso")
	}
	if cambio.Tipo != cambios.Modificado || cambio.AfectaProgreso {
		t.Errorf("cambio inesperado: %+v", cambio)
	}
}

// Añadir un obligatorio cambia lo que hay que hacer para aprobar: es mayor.
func TestAgregarUnObligatorioEsUnCambioMayor(t *testing.T) {
	antes := arbol(recurso(lectura, "Lectura", true, true))
	despues := arbol(
		recurso(lectura, "Lectura", true, true),
		recurso(video, "Clase grabada", true, true),
	)

	c := cambios.Comparar(antes, despues)
	if c.Alcance != cambios.Mayor {
		t.Fatalf("alcance %q, se esperaba mayor", c.Alcance)
	}
	if len(c.ObligatoriosAgregados) != 1 || c.ObligatoriosAgregados[0] != video {
		t.Errorf("no se identificó el obligatorio agregado: %+v", c.ObligatoriosAgregados)
	}
	cambio, _ := buscar(c, video)
	if !cambio.AfectaProgreso {
		t.Error("el alta de un obligatorio debería marcarse como que afecta al progreso")
	}
}

// Añadir material opcional no cambia lo exigido.
func TestAgregarUnRecursoOpcionalEsMenor(t *testing.T) {
	antes := arbol(recurso(lectura, "Lectura", true, true))
	despues := arbol(
		recurso(lectura, "Lectura", true, true),
		recurso(video, "Material complementario", true, false),
	)

	c := cambios.Comparar(antes, despues)
	if c.Alcance != cambios.Menor {
		t.Errorf("alcance %q, se esperaba menor", c.Alcance)
	}
	if len(c.ObligatoriosAgregados) != 0 {
		t.Errorf("un opcional no debería contar como obligatorio agregado: %+v", c.ObligatoriosAgregados)
	}
}

// Quitarle la obligatoriedad a un recurso también cambia la vara de medir.
func TestQuitarLaObligatoriedadEsMayorYSeExplica(t *testing.T) {
	antes := arbol(recurso(lectura, "Lectura", true, true))
	despues := arbol(recurso(lectura, "Lectura", true, false))

	c := cambios.Comparar(antes, despues)
	if c.Alcance != cambios.Mayor {
		t.Fatalf("alcance %q, se esperaba mayor", c.Alcance)
	}
	cambio, _ := buscar(c, lectura)
	if cambio.Detalle != "deja de ser obligatorio" {
		t.Errorf("el detalle no explica el cambio: %q", cambio.Detalle)
	}
	if len(c.ObligatoriosEliminados) != 1 {
		t.Errorf("no se identificó el obligatorio que salió: %+v", c.ObligatoriosEliminados)
	}
}

// Ocultar un obligatorio lo saca del cálculo aunque el recurso siga existiendo.
func TestOcultarUnObligatorioLoSacaDelCalculo(t *testing.T) {
	antes := arbol(recurso(lectura, "Lectura", true, true))
	despues := arbol(recurso(lectura, "Lectura", false, true))

	c := cambios.Comparar(antes, despues)
	if c.Alcance != cambios.Mayor {
		t.Errorf("alcance %q, se esperaba mayor", c.Alcance)
	}
	if len(c.ObligatoriosEliminados) != 1 || c.ObligatoriosEliminados[0] != lectura {
		t.Errorf("ocultar no sacó el recurso del conjunto obligatorio: %+v", c.ObligatoriosEliminados)
	}
}

func TestEliminarUnRecursoSeListaComoEliminado(t *testing.T) {
	antes := arbol(recurso(lectura, "Lectura", true, true), recurso(video, "Clase", true, true))
	despues := arbol(recurso(lectura, "Lectura", true, true))

	c := cambios.Comparar(antes, despues)
	cambio, ok := buscar(c, video)
	if !ok {
		t.Fatal("no se listó el recurso eliminado")
	}
	if cambio.Tipo != cambios.Eliminado || !cambio.AfectaProgreso {
		t.Errorf("cambio inesperado: %+v", cambio)
	}
}

// Reordenar no altera el progreso: se mide por stable_id, no por posición.
func TestReordenarNoAfectaAlProgreso(t *testing.T) {
	antes := arbol(recurso(lectura, "Lectura", true, true))
	movido := recurso(lectura, "Lectura", true, true)
	movido.Position = 5
	despues := arbol(movido)

	c := cambios.Comparar(antes, despues)
	if c.Alcance != cambios.Menor {
		t.Fatalf("alcance %q, se esperaba menor", c.Alcance)
	}
	cambio, ok := buscar(c, lectura)
	if !ok {
		t.Fatal("no se listó el movimiento")
	}
	if cambio.Tipo != cambios.Movido || cambio.AfectaProgreso {
		t.Errorf("cambio inesperado: %+v", cambio)
	}
}

// Un curso que nunca se publicó no tiene inscritos ni progreso, así que su
// primer borrador no es una actualización mayor por mucho contenido que traiga.
func TestElPrimerBorradorDeUnCursoNuevoNoEsMayor(t *testing.T) {
	c := cambios.Comparar(nil, arbol(recurso(lectura, "Lectura", true, true)))
	if c.Alcance != cambios.Menor {
		t.Errorf("alcance %q, se esperaba menor", c.Alcance)
	}
}

// La lista tiene que salir siempre igual: una respuesta que cambia de orden
// entre llamadas no se puede comparar ni revisar.
func TestElOrdenDeLosCambiosEsEstable(t *testing.T) {
	antes := arbol(recurso(lectura, "A", true, true))
	despues := arbol(
		recurso(lectura, "A modificada", true, true),
		recurso(video, "B", true, true),
		recurso(uuid.MustParse("55555555-5555-5555-5555-555555555555"), "C", true, false),
	)

	primera := cambios.Comparar(antes, despues)
	for i := 0; i < 20; i++ {
		otra := cambios.Comparar(antes, despues)
		if len(otra.Cambios) != len(primera.Cambios) {
			t.Fatalf("distinto número de cambios entre llamadas")
		}
		for j := range otra.Cambios {
			if otra.Cambios[j].StableID != primera.Cambios[j].StableID {
				t.Fatalf("el orden cambió entre llamadas en la posición %d", j)
			}
		}
	}
}
