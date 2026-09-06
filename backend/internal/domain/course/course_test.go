package course

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
)

func validResource() Resource {
	return Resource{ID: uuid.New(), StableID: uuid.New(), Type: ResourceText, Title: "Bienvenida", Visible: true, Required: true}
}

func validVersion() *Version {
	return &Version{
		Status:  VersionDraft,
		Title:   "Curso de prueba",
		Summary: "Un resumen",
		ApprovalMinScore:             60,
		ApprovalRequiredResourcesPct: 100,
		Modules: []Module{
			{
				Title: "Módulo 1",
				Units: []Unit{
					{Title: "Unidad 1", Resources: []Resource{validResource()}},
				},
			},
		},
	}
}

func TestValidateForPublish_Valid(t *testing.T) {
	v := validVersion()
	if err := v.ValidateForPublish(); err != nil {
		t.Fatalf("esperaba versión válida, obtuve: %v", err)
	}
}

func TestValidateForPublish_ListaExhaustiva(t *testing.T) {
	v := &Version{Status: VersionDraft} // sin título, resumen ni módulos

	err := v.ValidateForPublish()
	if err == nil {
		t.Fatal("esperaba error de validación")
	}
	var ve *ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("esperaba *ValidationError, obtuve %T", err)
	}
	if len(ve.Reasons) < 3 {
		t.Fatalf("esperaba una lista exhaustiva de motivos (>=3), obtuve %d: %v", len(ve.Reasons), ve.Reasons)
	}
}

func TestValidateForPublish_RechazaSiNoEsBorrador(t *testing.T) {
	v := validVersion()
	v.Status = VersionPublished

	err := v.ValidateForPublish()
	if err == nil {
		t.Fatal("esperaba error: solo un borrador puede publicarse")
	}
}

func TestValidateForPublish_RecursoDeVideoSinProcesar(t *testing.T) {
	v := validVersion()
	v.Modules[0].Units[0].Resources = append(v.Modules[0].Units[0].Resources, Resource{
		Type: ResourceVideo, Title: "Video", Visible: true, ProcessingStatus: ProcessingPending,
	})

	if err := v.ValidateForPublish(); err == nil {
		t.Fatal("esperaba error: recurso de video aún procesándose")
	}
}

func TestValidateForPublish_RecursoDeVideoListoOk(t *testing.T) {
	v := validVersion()
	v.Modules[0].Units[0].Resources = append(v.Modules[0].Units[0].Resources, Resource{
		Type: ResourceVideo, Title: "Video", Visible: true, ProcessingStatus: ProcessingReady,
	})

	if err := v.ValidateForPublish(); err != nil {
		t.Fatalf("esperaba versión válida con video listo, obtuve: %v", err)
	}
}

func TestValidateForPublish_UnidadSinRecursosVisibles(t *testing.T) {
	v := validVersion()
	v.Modules[0].Units[0].Resources[0].Visible = false

	if err := v.ValidateForPublish(); err == nil {
		t.Fatal("esperaba error: unidad sin recursos visibles")
	}
}

func TestValidateForPublish_ModuloSinUnidades(t *testing.T) {
	v := validVersion()
	v.Modules[0].Units = nil

	if err := v.ValidateForPublish(); err == nil {
		t.Fatal("esperaba error: módulo sin unidades")
	}
}

func TestPublish_TransicionaEstado(t *testing.T) {
	v := validVersion()
	now := time.Now().UTC()
	if err := v.Publish(now); err != nil {
		t.Fatalf("no se esperaba error: %v", err)
	}
	if v.Status != VersionPublished {
		t.Fatalf("esperaba estado published, obtuve %s", v.Status)
	}
	if v.PublishedAt == nil {
		t.Fatal("esperaba PublishedAt asignado")
	}

	if err := v.Publish(now); !errors.Is(err, ErrNotDraft) {
		t.Fatalf("esperaba ErrNotDraft al republicar, obtuve: %v", err)
	}
}
