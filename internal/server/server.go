package server

import (
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog"

	"github.com/Mark-0731/SwiftMail/internal/abuse"
	"github.com/Mark-0731/SwiftMail/internal/admin"
	"github.com/Mark-0731/SwiftMail/internal/auth"
	"github.com/Mark-0731/SwiftMail/internal/billing"
	"github.com/Mark-0731/SwiftMail/internal/config"
	"github.com/Mark-0731/SwiftMail/internal/domain"
	emailhandler "github.com/Mark-0731/SwiftMail/internal/email/handler"
	emailorchestrator "github.com/Mark-0731/SwiftMail/internal/email/orchestrator"
	emailrepo "github.com/Mark-0731/SwiftMail/internal/email/repository"
	"github.com/Mark-0731/SwiftMail/internal/infrastructure/cache"
	"github.com/Mark-0731/SwiftMail/internal/infrastructure/queue"
	"github.com/Mark-0731/SwiftMail/internal/server/middleware"
	"github.com/Mark-0731/SwiftMail/internal/suppression"
	tmpl "github.com/Mark-0731/SwiftMail/internal/template"
	"github.com/Mark-0731/SwiftMail/internal/tracking"
	"github.com/Mark-0731/SwiftMail/internal/user"
	"github.com/Mark-0731/SwiftMail/internal/warmup"
	"github.com/Mark-0731/SwiftMail/internal/webhook"
	"github.com/Mark-0731/SwiftMail/pkg/metrics"
	"github.com/Mark-0731/SwiftMail/pkg/ratelimit"
)

// New creates and configures the Fiber application with all routes.
func New(cfg *config.Config, db *pgxpool.Pool, rdb *redis.Client, asynqClient *asynq.Client, m *metrics.Metrics, logger zerolog.Logger) *fiber.App {
	app := fiber.New(fiber.Config{
		AppName:               "SwiftMail API",
		ReadTimeout:           cfg.Server.ReadTimeout,
		WriteTimeout:          cfg.Server.WriteTimeout,
		DisableStartupMessage: cfg.App.Env == "production",
	})

	// ─── Global Middleware ───────────────────────────────────────────────
	app.Use(recover.New())
	app.Use(middleware.RequestID())
	app.Use(middleware.CORS())
	app.Use(middleware.Logger(logger))
	app.Use(middleware.Metrics(m))

	// ─── Managers ────────────────────────────────────────────────────────
	jwtManager := auth.NewJWTManager(cfg.JWT.AccessSecret, cfg.JWT.RefreshSecret, cfg.JWT.AccessExpiry, cfg.JWT.RefreshExpiry)
	apiKeyManager := auth.NewAPIKeyManager(rdb)
	totpManager := auth.NewTOTPManager()
	rateLimiter := ratelimit.NewTokenBucket(rdb)

	// ─── Repositories ────────────────────────────────────────────────────
	authRepo := auth.NewPostgresRepository(db)
	domainRepo := domain.NewPostgresRepository(db)
	templateRepo := tmpl.NewPostgresRepository(db)
	emailRepo := emailrepo.NewPostgresRepository(db)
	suppressionRepo := suppression.NewPostgresRepository(db)
	webhookRepo := webhook.NewRepository(db)
	userRepo := user.NewRepository(db)

	// ─── Infrastructure Adapters ─────────────────────────────────────────
	cacheAdapter := cache.NewRedisCache(rdb, logger)
	queueAdapter := queue.NewAsynqQueue(asynqClient, logger)

	// ─── Services ────────────────────────────────────────────────────────
	authService := auth.NewService(authRepo, jwtManager, totpManager, apiKeyManager, logger)
	dnsChecker := domain.NewDNSChecker()
	domainService := domain.NewService(domainRepo, dnsChecker, rdb, logger)
	templateService := tmpl.NewService(templateRepo, logger)

	// Email orchestrator (no service layer)
	emailOrchestrator := emailorchestrator.NewOrchestrator(emailRepo, templateService, cacheAdapter, queueAdapter, rdb, logger)

	suppressionService := suppression.NewService(suppressionRepo, rdb, logger)
	webhookDispatcher := webhook.NewDispatcher(webhookRepo, logger)
	stripeService := billing.NewStripeService(
		cfg.Stripe.SecretKey,
		cfg.Stripe.WebhookSecret,
		cfg.Stripe.PublishableKey,
		cfg.Stripe.PriceIDStarter,
		cfg.Stripe.PriceIDPro,
		cfg.Stripe.SuccessURL,
		cfg.Stripe.CancelURL,
		logger,
	)
	billingService := billing.NewService(db, rdb, stripeService, logger)
	abuseDetector := abuse.NewDetector(db, rdb, logger)
	warmupScheduler := warmup.NewScheduler(db, logger)

	// ─── Handlers ────────────────────────────────────────────────────────
	authHandler := auth.NewHandler(authService)
	domainHandler := domain.NewHandler(domainService)
	templateHandler := tmpl.NewHandler(templateService)
	emailHandler := emailhandler.NewHandler(emailOrchestrator, logger)
	suppressionHandler := suppression.NewHandler(suppressionService)
	trackingHandler := tracking.NewHandler(asynqClient, rdb, logger)
	webhookHandler := webhook.NewHandler(webhookRepo, webhookDispatcher)
	billingHandler := billing.NewHandler(billingService)
	adminHandler := admin.NewHandler(userRepo)
	abuseHandler := abuse.NewHandler(abuseDetector)
	warmupHandler := warmup.NewHandler(warmupScheduler)

	// ─── Health ──────────────────────────────────────────────────────────
	app.Get("/health", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{"status": "ok", "service": "swiftmail-api"})
	})

	// ─── Public Routes (no auth) ─────────────────────────────────────────
	tracking.RegisterRoutes(app, trackingHandler)

	// Stripe webhook (public, no auth)
	billing.RegisterWebhookRoutes(app, billingHandler)

	// ─── API v1 ──────────────────────────────────────────────────────────
	v1 := app.Group("/v1")

	// Auth (public)
	auth.RegisterPublicRoutes(v1, authHandler)

	// ─── Authenticated Routes ────────────────────────────────────────────
	authenticated := v1.Group("", middleware.EitherAuth(jwtManager, apiKeyManager, rdb, logger))
	authenticated.Use(middleware.RateLimit(rateLimiter, cfg.RateLimit.PerSec, cfg.RateLimit.PerDay))

	// Auth (protected)
	auth.RegisterProtectedRoutes(authenticated, authHandler)

	// Domain routes
	domain.RegisterRoutes(authenticated, domainHandler)

	// Template routes
	tmpl.RegisterRoutes(authenticated, templateHandler)

	// Email send (with idempotency middleware)
	mailGroup := authenticated.Group("/mail")
	mailGroup.Use(middleware.Idempotency(rdb))
	mailGroup.Post("/send", emailHandler.Send)

	// Email log routes
	emailhandler.RegisterRoutes(authenticated, emailHandler)

	// Suppression routes
	suppression.RegisterRoutes(authenticated, suppressionHandler)

	// Webhook routes
	webhook.RegisterRoutes(authenticated, webhookHandler)

	// Billing routes
	billing.RegisterRoutes(authenticated, billingHandler)

	// ─── Admin (owner only) ──────────────────────────────────────────────
	adminGroup := v1.Group("", middleware.JWTAuth(jwtManager, logger), middleware.RequireRole("owner"))
	admin.RegisterRoutes(adminGroup, adminHandler)
	abuse.RegisterRoutes(adminGroup, abuseHandler)
	warmup.RegisterRoutes(adminGroup, warmupHandler)

	return app
}
