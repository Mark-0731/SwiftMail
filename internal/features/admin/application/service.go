package application

import (
	"context"

	"github.com/Mark-0731/SwiftMail/internal/features/user"
	"github.com/google/uuid"
)

// Service defines admin operations use-cases
type Service interface {
	// User management
	ListUsers(ctx context.Context, page, perPage int) ([]user.User, int64, error)
	SuspendUser(ctx context.Context, id uuid.UUID, reason string) error
	UnsuspendUser(ctx context.Context, id uuid.UUID) error

	// System health
	GetSystemHealth(ctx context.Context) (*SystemHealth, error)
}

// SystemHealth represents system health status
type SystemHealth struct {
	Status    string            `json:"status"`
	Services  map[string]string `json:"services"`
	Timestamp int64             `json:"timestamp"`
}

// AdminService implements Service interface
type AdminService struct {
	userRepo    user.Repository
	healthCheck HealthChecker
}

// HealthChecker checks health of system components
type HealthChecker interface {
	CheckSMTPPool(ctx context.Context) error
	CheckQueue(ctx context.Context) error
	CheckDatabase(ctx context.Context) error
}

// NewAdminService creates a new admin service
func NewAdminService(userRepo user.Repository, healthCheck HealthChecker) Service {
	return &AdminService{
		userRepo:    userRepo,
		healthCheck: healthCheck,
	}
}

// ListUsers retrieves paginated user list
func (s *AdminService) ListUsers(ctx context.Context, page, perPage int) ([]user.User, int64, error) {
	// Business logic: validate pagination
	if page < 1 {
		page = 1
	}
	if perPage < 1 || perPage > 100 {
		perPage = 50
	}

	return s.userRepo.ListAll(ctx, page, perPage)
}

// SuspendUser suspends a user account with audit trail
func (s *AdminService) SuspendUser(ctx context.Context, id uuid.UUID, reason string) error {
	// Business logic: validate user exists
	_, err := s.userRepo.GetByID(ctx, id)
	if err != nil {
		return err
	}

	// Business logic: cannot suspend already suspended users
	// (add check here if needed)

	// Execute suspension
	if err := s.userRepo.Suspend(ctx, id); err != nil {
		return err
	}

	// Business logic: create audit log (future enhancement)
	// s.auditRepo.Log(ctx, "user.suspended", id, reason)

	// Business logic: publish event (future enhancement)
	// s.eventBus.Publish(ctx, UserSuspendedEvent{UserID: id, Reason: reason})

	return nil
}

// UnsuspendUser reactivates a suspended user account
func (s *AdminService) UnsuspendUser(ctx context.Context, id uuid.UUID) error {
	// Business logic: validate user exists
	_, err := s.userRepo.GetByID(ctx, id)
	if err != nil {
		return err
	}

	// Execute unsuspension
	if err := s.userRepo.Unsuspend(ctx, id); err != nil {
		return err
	}

	// Business logic: create audit log (future enhancement)
	// s.auditRepo.Log(ctx, "user.unsuspended", id, "")

	return nil
}

// GetSystemHealth checks health of all system components
func (s *AdminService) GetSystemHealth(ctx context.Context) (*SystemHealth, error) {
	services := make(map[string]string)

	// Check SMTP pool
	if err := s.healthCheck.CheckSMTPPool(ctx); err != nil {
		services["smtp_pool"] = "unhealthy"
	} else {
		services["smtp_pool"] = "healthy"
	}

	// Check queue
	if err := s.healthCheck.CheckQueue(ctx); err != nil {
		services["queue"] = "unhealthy"
	} else {
		services["queue"] = "healthy"
	}

	// Check database
	if err := s.healthCheck.CheckDatabase(ctx); err != nil {
		services["database"] = "unhealthy"
	} else {
		services["database"] = "healthy"
	}

	// Determine overall status
	status := "operational"
	for _, s := range services {
		if s == "unhealthy" {
			status = "degraded"
			break
		}
	}

	return &SystemHealth{
		Status:    status,
		Services:  services,
		Timestamp: 0, // Add time.Now().Unix() if needed
	}, nil
}
