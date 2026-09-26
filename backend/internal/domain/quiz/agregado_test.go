package quiz_test

import (
	"testing"

	"github.com/google/uuid"

	"github.com/DES-SOLUCIONES-CLOUD/proyecto-1/backend/internal/domain/quiz"
)

var (
	p1 = uuid.MustParse("aaaaaaaa-0000-0000-0000-000000000001")
	oA = uuid.MustParse("bbbbbbbb-0000-0000-0000-000000000001")
	oB = uuid.MustParse("bbbbbbbb-0000-0000-0000-000000000002")
	oC = uuid.MustParse("bbbbbbbb-0000-0000-0000-000000000003")
)

// snapshotDeUnaPregunta arma el snapshot de un quiz de una sola pregunta con
// tres opciones, de las que la primera es la correcta.
func snapshotDeUnaPregunta() quiz.Snapshot {
	return quiz.Snapshot{
		QuestionOrder:  []uuid.UUID{p1},
		OptionOrder:    map[uuid.UUID][]uuid.UUID{p1: {oA, oB, oC}},
		CorrectAnswers: map[uuid.UUID][]uuid.UUID{p1: {oA}},
		Points:         map[uuid.UUID]float64{p1: 1},
		QuestionType:   map[uuid.UUID]quiz.QuestionType{p1: quiz.QuestionSingle},
	}
}

func textos() quiz.TextosDeQuiz {
	return quiz.TextosDeQuiz{
		Preguntas: map[uuid.UUID]string{p1: "¿Cuál es la fuente de verdad?"},
		Opciones:  map[uuid.UUID]string{oA: "PostgreSQL", oB: "Redis", oC: "El navegador"},
	}
}

func intento(seleccion []uuid.UUID, nota float64, aprobado bool) quiz.IntentoCalificado {
	return quiz.IntentoCalificado{
		Snapshot:   snapshotDeUnaPregunta(),
		Respuestas: map[uuid.UUID][]uuid.UUID{p1: seleccion},
		Score:      nota, Passed: aprobado,
	}
}

func TestSinIntentosElAgregadoNoInventaNada(t *testing.T) {
	r := quiz.Agregar(nil, 0, textos())
	if r.Intentos != 0 || r.NotaMedia != 0 || len(r.Preguntas) != 0 {
		t.Errorf("agregado inesperado sobre cero intentos: %+v", r)
	}
}

func TestLaDistribucionDeOpcionesDelataUnDistractorAmbiguo(t *testing.T) {
	intentos := []quiz.IntentoCalificado{
		intento([]uuid.UUID{oA}, 100, true),
		intento([]uuid.UUID{oB}, 0, false),
		intento([]uuid.UUID{oB}, 0, false),
		intento([]uuid.UUID{oB}, 0, false),
	}
	r := quiz.Agregar(intentos, 4, textos())

	if len(r.Preguntas) != 1 {
		t.Fatalf("se esperaba una pregunta: %+v", r.Preguntas)
	}
	pregunta := r.Preguntas[0]
	if pregunta.PromptMD != "¿Cuál es la fuente de verdad?" {
		t.Errorf("el enunciado no llegó al informe: %q", pregunta.PromptMD)
	}
	if pregunta.Aciertos != 1 || pregunta.TasaAcierto != 25 {
		t.Errorf("aciertos %d, tasa %v; se esperaba 1 y 25", pregunta.Aciertos, pregunta.TasaAcierto)
	}

	porOpcion := map[uuid.UUID]quiz.ResumenDeOpcion{}
	for _, o := range pregunta.Opciones {
		porOpcion[o.StableID] = o
	}
	if b := porOpcion[oB]; b.Elegida != 3 || b.Porcentaje != 75 || b.EsCorrecta {
		t.Errorf("el distractor no se contabilizó: %+v", b)
	}
	if a := porOpcion[oA]; !a.EsCorrecta || a.Elegida != 1 {
		t.Errorf("la opción correcta no se contabilizó: %+v", a)
	}
	// Una opción que nadie eligió tiene que seguir apareciendo: su ausencia de
	// respuestas es información.
	if c, ok := porOpcion[oC]; !ok || c.Elegida != 0 {
		t.Errorf("la opción que nadie eligió no aparece: %+v", c)
	}
}

// Dejar una pregunta en blanco no es lo mismo que fallarla, y el informe tiene
// que distinguirlo: una que casi nadie contesta suele estar mal planteada.
func TestElEnBlancoSeDistingueDelFallo(t *testing.T) {
	intentos := []quiz.IntentoCalificado{
		intento([]uuid.UUID{oA}, 100, true),
		intento([]uuid.UUID{oB}, 0, false),
		intento(nil, 0, false),
	}
	r := quiz.Agregar(intentos, 3, textos())
	pregunta := r.Preguntas[0]

	if pregunta.Respondida != 2 || pregunta.EnBlanco != 1 {
		t.Errorf("respondida %d, en blanco %d; se esperaba 2 y 1", pregunta.Respondida, pregunta.EnBlanco)
	}
	// La tasa se mide sobre quienes la vieron, no sobre quienes la contestaron.
	if pregunta.TasaAcierto != 33.33 {
		t.Errorf("tasa de acierto %v, se esperaba 33.33 (1 de 3 que la vieron)", pregunta.TasaAcierto)
	}
}

func TestLaMedianaResisteLaDistribucionBimodal(t *testing.T) {
	intentos := []quiz.IntentoCalificado{
		intento([]uuid.UUID{oA}, 100, true),
		intento([]uuid.UUID{oA}, 100, true),
		intento([]uuid.UUID{oB}, 0, false),
		intento([]uuid.UUID{oB}, 0, false),
		intento([]uuid.UUID{oB}, 0, false),
	}
	r := quiz.Agregar(intentos, 5, textos())

	if r.NotaMedia != 40 {
		t.Errorf("media %v, se esperaba 40", r.NotaMedia)
	}
	if r.NotaMediana != 0 {
		t.Errorf("mediana %v, se esperaba 0", r.NotaMediana)
	}
	if r.TasaAprobado != 40 {
		t.Errorf("tasa de aprobado %v, se esperaba 40", r.TasaAprobado)
	}
}

// Contar intentos no es contar personas: con tres intentos por cabeza, la
// población se triplicaría.
func TestLosEstudiantesNoSeConfundenConLosIntentos(t *testing.T) {
	intentos := []quiz.IntentoCalificado{
		intento([]uuid.UUID{oB}, 0, false),
		intento([]uuid.UUID{oB}, 0, false),
		intento([]uuid.UUID{oA}, 100, true),
	}
	r := quiz.Agregar(intentos, 1, textos())

	if r.Intentos != 3 || r.Estudiantes != 1 {
		t.Errorf("intentos %d, estudiantes %d; se esperaba 3 y 1", r.Intentos, r.Estudiantes)
	}
}

// Un quiz editado después de presentarse deja intentos con claves distintas.
// Cada uno se cuenta contra lo que vio, o el informe diría que alguien falló
// una pregunta que en su momento acertó.
func TestCadaIntentoSeCuentaContraElSnapshotQueVio(t *testing.T) {
	original := intento([]uuid.UUID{oA}, 100, true)

	// Más tarde el profesor cambia la respuesta correcta a oB.
	corregido := snapshotDeUnaPregunta()
	corregido.CorrectAnswers = map[uuid.UUID][]uuid.UUID{p1: {oB}}
	posterior := quiz.IntentoCalificado{
		Snapshot:   corregido,
		Respuestas: map[uuid.UUID][]uuid.UUID{p1: {oB}},
		Score:      100, Passed: true,
	}

	r := quiz.Agregar([]quiz.IntentoCalificado{original, posterior}, 2, textos())
	pregunta := r.Preguntas[0]

	// Los dos acertaron, cada uno contra su propia clave.
	if pregunta.Aciertos != 2 {
		t.Errorf("aciertos %d, se esperaba 2: cada intento contra el snapshot que vio", pregunta.Aciertos)
	}
}

// El orden de preguntas y opciones tiene que ser estable entre llamadas: un
// informe que se reordena solo no se puede leer ni comparar.
func TestElOrdenDelInformeEsEstable(t *testing.T) {
	intentos := []quiz.IntentoCalificado{
		intento([]uuid.UUID{oA}, 100, true),
		intento([]uuid.UUID{oC}, 0, false),
	}
	primera := quiz.Agregar(intentos, 2, textos())
	for i := 0; i < 20; i++ {
		otra := quiz.Agregar(intentos, 2, textos())
		for j := range otra.Preguntas[0].Opciones {
			if otra.Preguntas[0].Opciones[j].StableID != primera.Preguntas[0].Opciones[j].StableID {
				t.Fatalf("el orden de las opciones cambió entre llamadas")
			}
		}
	}
}
