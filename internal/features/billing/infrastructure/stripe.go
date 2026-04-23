package infrastructure

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"github.com/stripe/stripe-go/v85"
	"github.com/stripe/stripe-go/v85/customer"
	"github.com/stripe/stripe-go/v85/paymentintent"
)

// StripeService handles all Stripe operations.
type StripeService struct {
	secretKey      string
	publishableKey string
	logger         zerolog.Logger
}

// NewStripeService creates a new Stripe service.
func NewStripeService(
	secretKey, publishableKey string,
	logger zerolog.Logger,
) *StripeService {
	stripe.Key = secretKey
	return &StripeService{
		secretKey:      secretKey,
		publishableKey: publishableKey,
		logger:         logger,
	}
}

// CreateCustomer creates a Stripe customer for a user.
func (s *StripeService) CreateCustomer(ctx context.Context, userID uuid.UUID, email, name string) (string, error) {
	params := &stripe.CustomerParams{
		Email: stripe.String(email),
		Name:  stripe.String(name),
		Metadata: map[string]string{
			"user_id": userID.String(),
		},
	}

	cust, err := customer.New(params)
	if err != nil {
		s.logger.Error().Err(err).Str("user_id", userID.String()).Msg("failed to create Stripe customer")
		return "", fmt.Errorf("failed to create customer: %w", err)
	}

	s.logger.Info().Str("user_id", userID.String()).Str("customer_id", cust.ID).Msg("Stripe customer created")
	return cust.ID, nil
}

// CreatePaymentIntent creates a one-time payment intent for credit purchase.
func (s *StripeService) CreatePaymentIntent(ctx context.Context, customerID string, amountUSD int64, credits int64, userID uuid.UUID) (*stripe.PaymentIntent, error) {
	params := &stripe.PaymentIntentParams{
		Amount:   stripe.Int64(amountUSD * 100), // convert dollars to cents
		Currency: stripe.String(string(stripe.CurrencyUSD)),
		Customer: stripe.String(customerID),
		Metadata: map[string]string{
			"user_id": userID.String(),
			"type":    "credit_purchase",
			"credits": fmt.Sprintf("%d", credits),
		},
		AutomaticPaymentMethods: &stripe.PaymentIntentAutomaticPaymentMethodsParams{
			Enabled: stripe.Bool(true),
		},
	}

	pi, err := paymentintent.New(params)
	if err != nil {
		s.logger.Error().Err(err).Str("user_id", userID.String()).Msg("failed to create payment intent")
		return nil, fmt.Errorf("failed to create payment intent: %w", err)
	}

	s.logger.Info().
		Str("user_id", userID.String()).
		Str("payment_intent_id", pi.ID).
		Int64("amount_usd", amountUSD).
		Int64("credits", credits).
		Msg("payment intent created")
	return pi, nil
}

// GetPaymentIntent retrieves a payment intent to check its status.
func (s *StripeService) GetPaymentIntent(ctx context.Context, paymentIntentID string) (*stripe.PaymentIntent, error) {
	pi, err := paymentintent.Get(paymentIntentID, nil)
	if err != nil {
		s.logger.Error().Err(err).Str("payment_intent_id", paymentIntentID).Msg("failed to get payment intent")
		return nil, fmt.Errorf("failed to get payment intent: %w", err)
	}
	return pi, nil
}

// GetPublishableKey returns the Stripe publishable key for frontend.
func (s *StripeService) GetPublishableKey() string {
	return s.publishableKey
}
