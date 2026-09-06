// Package admin implementa la gestión administrativa de cuentas, roles,
// estados y auditoría, protegiendo siempre al último administrador activo.
package admin

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"

	"github.com/DES-SOLUCIONES-CLOUD/proyecto-1/backend/internal/domain/user"
	"github.com/DES-SOLUCIONES-CLOUD/proyecto-1/backend/internal/platform/postgres"
)

type Service struct {
	users *postgres.UserRepo
}

func NewService(users *postgres.UserRepo) *Service {
	return &Service{users: users}
}

func (s *Service) ListUsers(ctx context.Context, f postgres.ListUsersFilter) ([]*user.User, error) {
	return s.users.List(ctx, f)
}

// UpdateRole cambia el rol de un usuario, rechazando la operación si dejaría
// al sistema sin administradores activos.
func (s *Service) UpdateRole(ctx context.Context, actor *user.User, targetID uuid.UUID, newRole user.Role) error {
	if !newRole.Valid() {
		return user.ErrAccountNotActive
	}
	target, err := s.users.GetByID(ctx, targetID)
	if err != nil {
		return err
	}
	if target.Role == user.RoleAdmin && newRole != user.RoleAdmin {
		if err := s.assertNotLastActiveAdmin(ctx, target); err != nil {
			return err
		}
	}
	target.Role = newRole
	target.UpdatedAt = time.Now().UTC()
	if err := s.users.Update(ctx, target); err != nil {
		return err
	}
	return s.audit(ctx, actor, "user.role_updated", target.ID, map[string]any{"new_role": newRole})
}

// UpdateStatus activa o suspende una cuenta, protegiendo al último
// administrador activo y revocando sus sesiones si se suspende.
func (s *Service) UpdateStatus(ctx context.Context, actor *user.User, targetID uuid.UUID, newStatus user.Status) error {
	target, err := s.users.GetByID(ctx, targetID)
	if err != nil {
		return err
	}
	if target.Role == user.RoleAdmin && newStatus != user.StatusActive {
		if err := s.assertNotLastActiveAdmin(ctx, target); err != nil {
			return err
		}
	}
	now := time.Now().UTC()
	target.Status = newStatus
	target.UpdatedAt = now
	if err := s.users.Update(ctx, target); err != nil {
		return err
	}
	if newStatus == user.StatusSuspended {
		if err := s.users.RevokeAllSessionsForUser(ctx, target.ID, now); err != nil {
			return err
		}
	}
	return s.audit(ctx, actor, "user.status_updated", target.ID, map[string]any{"new_status": newStatus})
}

func (s *Service) assertNotLastActiveAdmin(ctx context.Context, target *user.User) error {
	if target.Status != user.StatusActive {
		return nil
	}
	count, err := s.users.CountActiveAdmins(ctx)
	if err != nil {
		return err
	}
	if count <= 1 {
		return user.ErrLastAdminProtected
	}
	return nil
}

func (s *Service) audit(ctx context.Context, actor *user.User, action string, entityID uuid.UUID, metadata map[string]any) error {
	meta, err := json.Marshal(metadata)
	if err != nil {
		return err
	}
	var actorID *uuid.UUID
	if actor != nil {
		actorID = &actor.ID
	}
	return s.users.InsertAudit(ctx, postgres.AuditEntry{
		ActorUserID: actorID,
		Action:      action,
		EntityType:  "user",
		EntityID:    entityID.String(),
		Metadata:    meta,
	})
}
