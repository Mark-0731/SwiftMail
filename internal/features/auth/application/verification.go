package application

import (
	"context"

	"github.com/google/uuid"
)

// SendVerificationEmail delegates to verification service
func (s *service) SendVerificationEmail(ctx context.Context, userID uuid.UUID) error {
	return s.verificationSvc.SendVerificationEmail(ctx, userID)
}

// VerifyEmail delegates to verification service
func (s *service) VerifyEmail(ctx context.Context, token string) error {
	return s.verificationSvc.VerifyEmail(ctx, token)
}
