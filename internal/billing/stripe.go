package billing

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"github.com/stripe/stripe-go/v85"
	"github.com/stripe/stripe-go/v85/checkout/session"
	"github.com/stripe/stripe-go/v85/customer"
	"github.com/stripe/stripe-go/v85/paymentintent"
	"github.com/stripe/stripe-go/v85/subscription"
	"github.com/stripe/stripe-go/v85/webhook"
)

// StripeService handles all Stripe operations.
type StripeService struct {
	secretKey      string
	webhookSecret  string
	publishableKey string
	priceIDStarter string
	priceIDPro     string
	successURL     string
	cancelURL      string
	logger         zerolog.Logger
}

// NewStripeService creates a new Stripe service.
func NewStripeService(
	secretKey, webhookSecret, publishableKey, priceIDStarter, priceIDPro, successURL, cancelURL string,
	logger zerolog.Logger,
) *StripeService {
	stripe.Key = secretKey
	return &StripeService{
		secretKey:      secretKey,
		webhookSecret:  webhookSecret,
		publishableKey: publishableKey,
		priceIDStarter: priceIDStarter,
		priceIDPro:     priceIDPro,
		successURL:     successURL,
		cancelURL:      cancelURL,
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

// CreateCheckoutSession creates a Stripe Checkout session for subscription.
func (s *StripeService) CreateCheckoutSession(ctx context.Context, customerID, priceID string, userID uuid.UUID) (*stripe.CheckoutSession, error) {
	params := &stripe.CheckoutSessionParams{
		Customer: stripe.String(customerID),
		Mode:     stripe.String(string(stripe.CheckoutSessionModeSubscription)),
		LineItems: []*stripe.CheckoutSessionLineItemParams{
			{
				Price:    stripe.String(priceID),
				Quantity: stripe.Int64(1),
			},
		},
		SuccessURL: stripe.String(s.successURL + "?session_id={CHECKOUT_SESSION_ID}"),
		CancelURL:  stripe.String(s.cancelURL),
		Metadata: map[string]string{
			"user_id": userID.String(),
		},
		SubscriptionData: &stripe.CheckoutSessionSubscriptionDataParams{
			Metadata: map[string]string{
				"user_id": userID.String(),
			},
		},
	}

	sess, err := session.New(params)
	if err != nil {
		s.logger.Error().Err(err).Str("user_id", userID.String()).Msg("failed to create checkout session")
		return nil, fmt.Errorf("failed to create checkout session: %w", err)
	}

	s.logger.Info().Str("user_id", userID.String()).Str("session_id", sess.ID).Msg("checkout session created")
	return sess, nil
}

// CreatePaymentIntent creates a one-time payment intent for credit purchase.
func (s *StripeService) CreatePaymentIntent(ctx context.Context, customerID string, amount int64, userID uuid.UUID) (*stripe.PaymentIntent, error) {
	params := &stripe.PaymentIntentParams{
		Amount:   stripe.Int64(amount), // in cents
		Currency: stripe.String(string(stripe.CurrencyUSD)),
		Customer: stripe.String(customerID),
		Metadata: map[string]string{
			"user_id": userID.String(),
			"type":    "credit_purchase",
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

	s.logger.Info().Str("user_id", userID.String()).Str("payment_intent_id", pi.ID).Msg("payment intent created")
	return pi, nil
}

// CancelSubscription cancels a Stripe subscription.
func (s *StripeService) CancelSubscription(ctx context.Context, subscriptionID string) error {
	params := &stripe.SubscriptionParams{
		CancelAtPeriodEnd: stripe.Bool(true),
	}

	_, err := subscription.Update(subscriptionID, params)
	if err != nil {
		s.logger.Error().Err(err).Str("subscription_id", subscriptionID).Msg("failed to cancel subscription")
		return fmt.Errorf("failed to cancel subscription: %w", err)
	}

	s.logger.Info().Str("subscription_id", subscriptionID).Msg("subscription cancelled")
	return nil
}

// GetSubscription retrieves a Stripe subscription.
func (s *StripeService) GetSubscription(ctx context.Context, subscriptionID string) (*stripe.Subscription, error) {
	sub, err := subscription.Get(subscriptionID, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get subscription: %w", err)
	}
	return sub, nil
}

// ConstructWebhookEvent constructs and verifies a Stripe webhook event.
func (s *StripeService) ConstructWebhookEvent(payload []byte, signature string) (stripe.Event, error) {
	event, err := webhook.ConstructEvent(payload, signature, s.webhookSecret)
	if err != nil {
		s.logger.Error().Err(err).Msg("failed to verify webhook signature")
		return event, fmt.Errorf("webhook signature verification failed: %w", err)
	}
	return event, nil
}

// GetPublishableKey returns the Stripe publishable key for frontend.
func (s *StripeService) GetPublishableKey() string {
	return s.publishableKey
}

// GetPriceID returns the Stripe price ID for a plan.
func (s *StripeService) GetPriceID(planID string) string {
	switch planID {
	case "starter":
		return s.priceIDStarter
	case "pro":
		return s.priceIDPro
	default:
		return ""
	}
}
