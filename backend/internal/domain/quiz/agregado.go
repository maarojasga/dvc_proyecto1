package quiz

import (
	"sort"

	"github.com/google/uuid"
)

// Agregación de resultados por evaluación (alcance opcional 5.2).
//
// La agregación se hace sobre el snapshot de cada intento y no sobre la
// definición vigente del quiz, y esa decisión ordena todo el archivo. Un quiz
// se puede editar después de que la gente lo haya presentado: si se contara
// contra la definición actual, una pregunta reformulada haría aparecer como
// fallos respuestas que en su momento fueron correctas, y el informe diría
// algo que nunca ocurrió.
//
// La consecuencia es que dos intentos pueden haber visto preguntas distintas
// bajo el mismo stable_id. El agregado lo asume: cuenta cada intento contra lo
// que ese intento vio, y el enunciado que muestra es el del último snapshot
// conocido, que es lo más cercano a "la pregunta tal como está hoy".

// IntentoCalificado es lo mínimo que la agregación necesita de un intento ya
// cerrado.
type IntentoCalificado struct {
	Snapshot Snapshot
	// Respuestas es lo que el estudiante marcó: pregunta -> opciones.
	Respuestas map[uuid.UUID][]uuid.UUID
	Score      float64
	Passed     bool
}

// ResumenDeQuiz son los resultados agregados de una evaluación.
type ResumenDeQuiz struct {
	Intentos int `json:"intentos"`
	// Estudiantes cuenta inscripciones distintas, no intentos: un quiz con
	// tres intentos por persona multiplicaría por tres la población si se
	// contaran intentos.
	Estudiantes  int                 `json:"estudiantes"`
	NotaMedia    float64             `json:"nota_media"`
	NotaMediana  float64             `json:"nota_mediana"`
	TasaAprobado float64             `json:"tasa_aprobado"`
	Preguntas    []ResumenDePregunta `json:"preguntas"`
}

// ResumenDePregunta mide qué tan bien funcionó una pregunta.
type ResumenDePregunta struct {
	StableID uuid.UUID `json:"stable_id"`
	PromptMD string    `json:"prompt_md"`
	// Respondida es cuántos intentos la contestaron. Puede ser menor que el
	// total: dejar una en blanco es una respuesta en sí, y distinguirlo de
	// fallarla es justamente lo que le sirve al profesor.
	Respondida int `json:"respondida"`
	EnBlanco   int `json:"en_blanco"`
	Aciertos   int `json:"aciertos"`
	// TasaAcierto se calcula sobre los intentos que la vieron, no sobre los
	// que la respondieron: una pregunta que casi nadie contesta es tan
	// problemática como una que casi nadie acierta.
	TasaAcierto float64           `json:"tasa_acierto"`
	Opciones    []ResumenDeOpcion `json:"opciones"`
}

// ResumenDeOpcion es cuánta gente eligió cada alternativa.
//
// Es lo que delata un distractor mal escrito: una opción incorrecta que elige
// la mitad de la clase suele ser una pregunta ambigua, no media clase que no
// estudió.
type ResumenDeOpcion struct {
	StableID   uuid.UUID `json:"stable_id"`
	TextMD     string    `json:"text_md"`
	EsCorrecta bool      `json:"es_correcta"`
	Elegida    int       `json:"elegida"`
	Porcentaje float64   `json:"porcentaje"`
}

// TextosDeQuiz son los enunciados con que presentar el agregado.
//
// Vienen aparte porque el snapshot guarda identificadores y la clave, no los
// textos: el informe los toma de la definición vigente, que es la que el
// profesor está mirando.
type TextosDeQuiz struct {
	Preguntas map[uuid.UUID]string
	Opciones  map[uuid.UUID]string
}

// Agregar calcula los resultados de una evaluación.
//
// `estudiantes` se pasa por fuera porque el número de personas distintas lo
// sabe quien consultó los intentos, no esta función.
func Agregar(intentos []IntentoCalificado, estudiantes int, textos TextosDeQuiz) ResumenDeQuiz {
	out := ResumenDeQuiz{Intentos: len(intentos), Estudiantes: estudiantes, Preguntas: []ResumenDePregunta{}}
	if len(intentos) == 0 {
		return out
	}

	notas := make([]float64, 0, len(intentos))
	aprobados := 0
	for _, it := range intentos {
		notas = append(notas, it.Score)
		if it.Passed {
			aprobados++
		}
	}
	out.NotaMedia = redondear(media(notas))
	out.NotaMediana = redondear(mediana(notas))
	out.TasaAprobado = redondear(float64(aprobados) * 100 / float64(len(intentos)))

	out.Preguntas = agregarPreguntas(intentos, textos)
	return out
}

// acumulador reúne lo que se sabe de una pregunta mientras se recorren los
// intentos.
type acumulador struct {
	vista      int
	respondida int
	aciertos   int
	// orden conserva las opciones tal como aparecieron, para que el informe no
	// las liste en el orden arbitrario de un mapa.
	orden    []uuid.UUID
	elegida  map[uuid.UUID]int
	correcta map[uuid.UUID]bool
	primeraE int // posición en que se vio por primera vez, para ordenar
}

func agregarPreguntas(intentos []IntentoCalificado, textos TextosDeQuiz) []ResumenDePregunta {
	acums := map[uuid.UUID]*acumulador{}
	var orden []uuid.UUID

	for _, it := range intentos {
		for _, preguntaID := range it.Snapshot.QuestionOrder {
			a, ok := acums[preguntaID]
			if !ok {
				a = &acumulador{elegida: map[uuid.UUID]int{}, correcta: map[uuid.UUID]bool{}, primeraE: len(orden)}
				acums[preguntaID] = a
				orden = append(orden, preguntaID)
			}
			a.vista++

			for _, opcionID := range it.Snapshot.OptionOrder[preguntaID] {
				if _, visto := a.elegida[opcionID]; !visto {
					a.elegida[opcionID] = 0
					a.orden = append(a.orden, opcionID)
				}
			}
			for _, correcta := range it.Snapshot.CorrectAnswers[preguntaID] {
				a.correcta[correcta] = true
			}

			seleccion := it.Respuestas[preguntaID]
			if len(seleccion) == 0 {
				continue
			}
			a.respondida++
			for _, opcionID := range seleccion {
				a.elegida[opcionID]++
			}
			if mismoConjunto(seleccion, it.Snapshot.CorrectAnswers[preguntaID]) {
				a.aciertos++
			}
		}
	}

	out := make([]ResumenDePregunta, 0, len(orden))
	for _, preguntaID := range orden {
		a := acums[preguntaID]
		r := ResumenDePregunta{
			StableID:   preguntaID,
			PromptMD:   textos.Preguntas[preguntaID],
			Respondida: a.respondida,
			EnBlanco:   a.vista - a.respondida,
			Aciertos:   a.aciertos,
		}
		if a.vista > 0 {
			r.TasaAcierto = redondear(float64(a.aciertos) * 100 / float64(a.vista))
		}
		for _, opcionID := range a.orden {
			o := ResumenDeOpcion{
				StableID:   opcionID,
				TextMD:     textos.Opciones[opcionID],
				EsCorrecta: a.correcta[opcionID],
				Elegida:    a.elegida[opcionID],
			}
			if a.vista > 0 {
				o.Porcentaje = redondear(float64(o.Elegida) * 100 / float64(a.vista))
			}
			r.Opciones = append(r.Opciones, o)
		}
		if r.Opciones == nil {
			r.Opciones = []ResumenDeOpcion{}
		}
		out = append(out, r)
	}
	return out
}

// mismoConjunto compara selección y clave sin importar el orden ni los
// repetidos, igual que hace la calificación.
func mismoConjunto(a, b []uuid.UUID) bool {
	sa, sb := aConjunto(a), aConjunto(b)
	if len(sa) != len(sb) {
		return false
	}
	for k := range sa {
		if _, ok := sb[k]; !ok {
			return false
		}
	}
	return true
}

func aConjunto(ids []uuid.UUID) map[uuid.UUID]struct{} {
	out := make(map[uuid.UUID]struct{}, len(ids))
	for _, id := range ids {
		out[id] = struct{}{}
	}
	return out
}

func media(xs []float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	var suma float64
	for _, x := range xs {
		suma += x
	}
	return suma / float64(len(xs))
}

// mediana dice más que la media cuando la distribución es bimodal, que es el
// caso típico de un quiz: quien estudió y quien no.
func mediana(xs []float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	ordenadas := append([]float64(nil), xs...)
	sort.Float64s(ordenadas)
	medio := len(ordenadas) / 2
	if len(ordenadas)%2 == 1 {
		return ordenadas[medio]
	}
	return (ordenadas[medio-1] + ordenadas[medio]) / 2
}

// redondear deja dos decimales: más precisión en un porcentaje sobre una
// muestra de estudiantes es ruido.
func redondear(x float64) float64 {
	return float64(int64(x*100+0.5)) / 100
}
