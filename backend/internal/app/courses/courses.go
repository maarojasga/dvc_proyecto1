// Package courses implementa los casos de uso de autoría: creación de
// borradores, edición de la jerarquía Módulo -> Unidad -> Recurso,
// previsualización, publicación con validación exhaustiva y catálogo.
package courses

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"

	domain "github.com/DES-SOLUCIONES-CLOUD/proyecto-1/backend/internal/domain/course"
	"github.com/DES-SOLUCIONES-CLOUD/proyecto-1/backend/internal/domain/user"
	"github.com/DES-SOLUCIONES-CLOUD/proyecto-1/backend/internal/platform/postgres"
)

var (
	ErrForbidden       = errors.New("courses: no tienes permiso sobre este curso")
	ErrVersionNotDraft = errors.New("courses: la versión no es un borrador editable")
)

type Service struct {
	repo *postgres.CourseRepo
}

func NewService(repo *postgres.CourseRepo) *Service {
	return &Service{repo: repo}
}

// CreateDraft crea un curso nuevo con su primera versión en borrador.
func (s *Service) CreateDraft(ctx context.Context, teacher *user.User, slug, title string) (*domain.Course, *domain.Version, error) {
	now := time.Now().UTC()
	c := &domain.Course{
		ID:        uuid.New(),
		TeacherID: teacher.ID,
		Slug:      slug,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := s.repo.CreateCourse(ctx, c); err != nil {
		return nil, nil, err
	}

	v := &domain.Version{
		ID:                           uuid.New(),
		CourseID:                     c.ID,
		VersionNumber:                1,
		Status:                       domain.VersionDraft,
		Title:                        title,
		Language:                     "es",
		ApprovalMinScore:             60,
		ApprovalRequiredResourcesPct: 100,
		CreatedAt:                    now,
		UpdatedAt:                    now,
	}
	if err := s.repo.CreateVersion(ctx, v); err != nil {
		return nil, nil, err
	}
	return c, v, nil
}

// CreateUpdateDraft crea la siguiente versión en borrador copiando módulos,
// unidades y recursos de la última versión, reutilizando los mismos
// StableID para conservar el progreso de los estudiantes ya inscritos.
func (s *Service) CreateUpdateDraft(ctx context.Context, actor *user.User, courseID uuid.UUID) (*domain.Version, error) {
	c, err := s.repo.GetCourse(ctx, courseID)
	if err != nil {
		return nil, err
	}
	if err := s.authorizeOwner(actor, c); err != nil {
		return nil, err
	}

	latestNum, err := s.repo.LatestVersionNumber(ctx, courseID)
	if err != nil {
		return nil, err
	}

	var sourceModules []domain.Module
	if c.CurrentPublishedVersionID != nil {
		sourceModules, err = s.repo.LoadTree(ctx, *c.CurrentPublishedVersionID)
		if err != nil {
			return nil, err
		}
	}

	now := time.Now().UTC()
	draft := domain.NewDraft(courseID, latestNum, now)
	if c.CurrentPublishedVersionID != nil {
		published, err := s.repo.GetVersion(ctx, *c.CurrentPublishedVersionID)
		if err != nil {
			return nil, err
		}
		draft.Title = published.Title
		draft.Summary = published.Summary
		draft.DescriptionMD = published.DescriptionMD
		draft.Category = published.Category
		draft.Level = published.Level
		draft.Language = published.Language
		draft.ApprovalMinScore = published.ApprovalMinScore
		draft.ApprovalRequiredResourcesPct = published.ApprovalRequiredResourcesPct
	}
	if err := s.repo.CreateVersion(ctx, draft); err != nil {
		return nil, err
	}

	for _, m := range sourceModules {
		newModule := &domain.Module{ID: uuid.New(), CourseVersionID: draft.ID, StableID: m.StableID, Title: m.Title, Position: m.Position}
		if err := s.repo.CreateModule(ctx, newModule); err != nil {
			return nil, err
		}
		for _, u := range m.Units {
			newUnit := &domain.Unit{ID: uuid.New(), ModuleID: newModule.ID, StableID: u.StableID, Title: u.Title, Position: u.Position}
			if err := s.repo.CreateUnit(ctx, draft.ID, newUnit); err != nil {
				return nil, err
			}
			for _, res := range u.Resources {
				newRes := res
				newRes.ID = uuid.New()
				newRes.UnitID = newUnit.ID
				if err := s.repo.CreateResource(ctx, draft.ID, &newRes); err != nil {
					return nil, err
				}
			}
		}
	}

	return draft, nil
}

func (s *Service) authorizeOwner(actor *user.User, c *domain.Course) error {
	if actor.IsAdmin() {
		return nil
	}
	if actor.IsTeacher() && actor.ID == c.TeacherID {
		return nil
	}
	return ErrForbidden
}

// GetOwnedVersion recupera una versión validando que el actor sea el
// profesor dueño del curso o un administrador.
func (s *Service) GetOwnedVersion(ctx context.Context, actor *user.User, versionID uuid.UUID) (*domain.Course, *domain.Version, error) {
	v, err := s.repo.GetVersion(ctx, versionID)
	if err != nil {
		return nil, nil, err
	}
	c, err := s.repo.GetCourse(ctx, v.CourseID)
	if err != nil {
		return nil, nil, err
	}
	if err := s.authorizeOwner(actor, c); err != nil {
		return nil, nil, err
	}
	return c, v, nil
}

// PreviewVersion carga el árbol completo de una versión (propia) para
// previsualización de autoría.
func (s *Service) PreviewVersion(ctx context.Context, actor *user.User, versionID uuid.UUID) (*domain.Version, error) {
	_, v, err := s.GetOwnedVersion(ctx, actor, versionID)
	if err != nil {
		return nil, err
	}
	modules, err := s.repo.LoadTree(ctx, v.ID)
	if err != nil {
		return nil, err
	}
	v.Modules = modules
	return v, nil
}

func requireDraft(v *domain.Version) error {
	if v.Status != domain.VersionDraft {
		return ErrVersionNotDraft
	}
	return nil
}

// editableVersion resuelve la versión comprobando de una vez propiedad y
// estado.
//
// Toda mutación de estructura pasa por aquí: una versión publicada es
// inmutable, y editarla exige despublicarla antes. Antes solo lo comprobaban
// UpdateMetadata y AddModule, así que el resto de operaciones podían alterar
// un curso ya publicado.
func (s *Service) editableVersion(ctx context.Context, actor *user.User, versionID uuid.UUID) (*domain.Version, error) {
	_, v, err := s.GetOwnedVersion(ctx, actor, versionID)
	if err != nil {
		return nil, err
	}
	if err := requireDraft(v); err != nil {
		return nil, err
	}
	return v, nil
}

func (s *Service) UpdateMetadata(ctx context.Context, actor *user.User, versionID uuid.UUID, patch domain.Version) error {
	v, err := s.editableVersion(ctx, actor, versionID)
	if err != nil {
		return err
	}
	v.Title = patch.Title
	v.Summary = patch.Summary
	v.DescriptionMD = patch.DescriptionMD
	v.Category = patch.Category
	v.Level = patch.Level
	v.Language = patch.Language
	v.ApprovalMinScore = patch.ApprovalMinScore
	v.ApprovalRequiredResourcesPct = patch.ApprovalRequiredResourcesPct
	v.UpdatedAt = time.Now().UTC()
	return s.repo.UpdateVersionMetadata(ctx, v)
}

func (s *Service) AddModule(ctx context.Context, actor *user.User, versionID uuid.UUID, title string, position int) (*domain.Module, error) {
	v, err := s.editableVersion(ctx, actor, versionID)
	if err != nil {
		return nil, err
	}
	m := &domain.Module{ID: uuid.New(), CourseVersionID: v.ID, StableID: uuid.New(), Title: title, Position: position}
	if err := s.repo.CreateModule(ctx, m); err != nil {
		return nil, err
	}
	return m, nil
}

func (s *Service) UpdateModule(ctx context.Context, actor *user.User, versionID, moduleID uuid.UUID, title string, position int) error {
	if _, err := s.editableVersion(ctx, actor, versionID); err != nil {
		return err
	}
	return s.repo.UpdateModule(ctx, versionID, &domain.Module{ID: moduleID, Title: title, Position: position})
}

func (s *Service) DeleteModule(ctx context.Context, actor *user.User, versionID, moduleID uuid.UUID) error {
	if _, err := s.editableVersion(ctx, actor, versionID); err != nil {
		return err
	}
	return s.repo.DeleteModule(ctx, versionID, moduleID)
}

func (s *Service) UpdateUnit(ctx context.Context, actor *user.User, versionID, unitID uuid.UUID, title string, position int) error {
	if _, err := s.editableVersion(ctx, actor, versionID); err != nil {
		return err
	}
	return s.repo.UpdateUnit(ctx, versionID, &domain.Unit{ID: unitID, Title: title, Position: position})
}

func (s *Service) DeleteUnit(ctx context.Context, actor *user.User, versionID, unitID uuid.UUID) error {
	if _, err := s.editableVersion(ctx, actor, versionID); err != nil {
		return err
	}
	return s.repo.DeleteUnit(ctx, versionID, unitID)
}

func (s *Service) AddUnit(ctx context.Context, actor *user.User, versionID, moduleID uuid.UUID, title string, position int) (*domain.Unit, error) {
	if _, err := s.editableVersion(ctx, actor, versionID); err != nil {
		return nil, err
	}
	u := &domain.Unit{ID: uuid.New(), ModuleID: moduleID, StableID: uuid.New(), Title: title, Position: position}
	if err := s.repo.CreateUnit(ctx, versionID, u); err != nil {
		return nil, err
	}
	return u, nil
}

func (s *Service) AddResource(ctx context.Context, actor *user.User, versionID uuid.UUID, res *domain.Resource) (*domain.Resource, error) {
	if _, err := s.editableVersion(ctx, actor, versionID); err != nil {
		return nil, err
	}
	if !res.Type.Valid() {
		return nil, errors.New("courses: tipo de recurso inválido")
	}
	res.ID = uuid.New()
	res.StableID = uuid.New()
	if res.ProcessingStatus == "" {
		res.ProcessingStatus = domain.ProcessingNone
	}
	if err := s.repo.CreateResource(ctx, versionID, res); err != nil {
		return nil, err
	}
	return res, nil
}

// GetResource recupera un recurso validando la propiedad de la versión a la
// que pertenece (a través de su unidad y módulo).
func (s *Service) GetResource(ctx context.Context, actor *user.User, versionID, resourceID uuid.UUID) (*domain.Resource, error) {
	if _, _, err := s.GetOwnedVersion(ctx, actor, versionID); err != nil {
		return nil, err
	}
	return s.repo.GetResource(ctx, versionID, resourceID)
}

func (s *Service) SetResourceObjectKey(ctx context.Context, actor *user.User, versionID, resourceID uuid.UUID, objectKey string, status domain.ProcessingStatus) error {
	if _, err := s.editableVersion(ctx, actor, versionID); err != nil {
		return err
	}
	return s.repo.SetResourceObjectKey(ctx, versionID, resourceID, objectKey, status)
}

func (s *Service) MarkResourceProcessingStatus(ctx context.Context, actor *user.User, versionID, resourceID uuid.UUID, status domain.ProcessingStatus) error {
	if _, _, err := s.GetOwnedVersion(ctx, actor, versionID); err != nil {
		return err
	}
	return s.repo.SetResourceProcessingStatus(ctx, versionID, resourceID, status)
}

func (s *Service) UpdateResource(ctx context.Context, actor *user.User, versionID uuid.UUID, res *domain.Resource) error {
	if _, err := s.editableVersion(ctx, actor, versionID); err != nil {
		return err
	}
	return s.repo.UpdateResource(ctx, versionID, res)
}

func (s *Service) DeleteResource(ctx context.Context, actor *user.User, versionID, resourceID uuid.UUID) error {
	if _, err := s.editableVersion(ctx, actor, versionID); err != nil {
		return err
	}
	return s.repo.DeleteResource(ctx, versionID, resourceID)
}

// PublishVersion valida exhaustivamente y publica una versión en borrador,
// despublicando atómicamente la versión previamente vigente si existía.
func (s *Service) PublishVersion(ctx context.Context, actor *user.User, versionID uuid.UUID) error {
	c, v, err := s.GetOwnedVersion(ctx, actor, versionID)
	if err != nil {
		return err
	}
	modules, err := s.repo.LoadTree(ctx, v.ID)
	if err != nil {
		return err
	}
	v.Modules = modules

	if err := v.ValidateForPublish(); err != nil {
		return err
	}

	now := time.Now().UTC()
	return s.repo.PublishVersionAtomic(ctx, c.ID, c.CurrentPublishedVersionID, v.ID, now)
}

// UnpublishVersion retira temporalmente la versión vigente para permitir su
// edición (regla obligatoria del MVP).
func (s *Service) UnpublishVersion(ctx context.Context, actor *user.User, courseID uuid.UUID) error {
	c, err := s.repo.GetCourse(ctx, courseID)
	if err != nil {
		return err
	}
	if err := s.authorizeOwner(actor, c); err != nil {
		return err
	}
	if c.CurrentPublishedVersionID == nil {
		return errors.New("courses: el curso no tiene una versión publicada")
	}
	return s.repo.UnpublishVersionAtomic(ctx, c.ID, *c.CurrentPublishedVersionID, time.Now().UTC())
}

func (s *Service) ListMine(ctx context.Context, teacher *user.User) ([]*domain.Course, error) {
	return s.repo.ListByTeacher(ctx, teacher.ID)
}

func (s *Service) ListCatalog(ctx context.Context, f postgres.CatalogFilter) (postgres.PaginaDeCatalogo, error) {
	return s.repo.ListCatalog(ctx, f)
}

// GetPublishedByCourseID devuelve la versión publicada vigente de un curso,
// con su árbol completo, para el catálogo y el visor de estudiante.
func (s *Service) GetPublishedByCourseID(ctx context.Context, courseID uuid.UUID) (*domain.Course, *domain.Version, error) {
	c, err := s.repo.GetCourse(ctx, courseID)
	if err != nil {
		return nil, nil, err
	}
	if c.CurrentPublishedVersionID == nil {
		return nil, nil, postgres.ErrNotFound
	}
	v, err := s.repo.GetVersion(ctx, *c.CurrentPublishedVersionID)
	if err != nil {
		return nil, nil, err
	}
	v.Modules, err = s.repo.LoadTree(ctx, v.ID)
	if err != nil {
		return nil, nil, err
	}
	return c, v, nil
}
