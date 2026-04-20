package server

import (
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog"

	"github.com/Mark-0731/SwiftMail/internal/config"
	abuseapp "github.com/Mark-0731/SwiftMail/internal/features/abuse/application"
	abusehttp "github.com/Mark-0731/SwiftMail/internal/features/abuse/transport/http"
	adminapp "github.com/Mark-0731/SwiftMail/internal/features/admin/application"
	admininfra "github.com/Mark-0731/SwiftMail/internal/features/admin/infrastructure"
	adminhttp "github.com/Mark-0731/SwiftMail/internal/features/admin/transport/http"
	authapp "github.com/Mark-0731/SwiftMail/internal/features/auth/application"
	authdomain "github.com/Mark-0731/SwiftMail/internal/features/auth/domain"
	authinfra "github.com/Mark-0731/SwiftMail/internal/features/auth/infrastructure"
	authhttp "github.com/Mark-0731/SwiftMail/internal/features/auth/transport/http"
	billingapp "github.com/Mark-0731/SwiftMail/internal/features/billing/application"
	billinginfra "github.com/Mark-0731/SwiftMail/internal/features/billing/infrastructure"
	billinghttp "github.com/Mark-0731/SwiftMail/internal/features/billing/transport/http"
	domainmgmtapp "github.com/Mark-0731/SwiftMail/internal/features/domainmgmt/application"
	domainmgmtdomain "github.com/Mark-0731/SwiftMail/internal/features/domainmgmt/domain"
	domainmgmtinfra "github.com/Mark-0731/SwiftMail/internal/features/domainmgmt/infrastructure"
	domainmgmthttp "github.com/Mark-0731/SwiftMail/internal/features/domainmgmt/transport/http"
	emailapp "github.com/Mark-0731/SwiftMail/internal/features/email/application"
	emailinfra "github.com/Mark-0731/SwiftMail/internal/features/email/infrastructure"
	emailhttp "github.com/Mark-0731/SwiftMail/internal/features/email/transport/http"
	suppressionapp "github.com/Mark-0731/SwiftMail/internal/features/suppression/application"
	suppressioninfra "github.com/Mark-0731/SwiftMail/internal/features/suppression/infrastructure"
	suppressionhttp "github.com/Mark-0731/SwiftMail/internal/features/suppression/transport/http"
	tmplapp "github.com/Mark-0731/SwiftMail/internal/features/template/application"
	tmplinfra "github.com/Mark-0731/SwiftMail/internal/features/template/infrastructure"
	tmplhttp "github.com/Mark-0731/SwiftMail/internal/features/template/transport/http"
	trackinghttp "github.com/Mark-0731/SwiftMail/internal/features/tracking/transport/http"
	userinfra "github.com/Mark-0731/SwiftMail/internal/features/user/infrastructure"
	verificationapp "github.com/Mark-0731/SwiftMail/internal/features/verification/application"
	verificationinfra "github.com/Mark-0731/SwiftMail/internal/features/verification/infrastructure"
	warmupapp "github.com/Mark-0731/SwiftMail/internal/features/warmup/application"
	warmuphttp "github.com/Mark-0731/SwiftMail/internal/features/warmup/transport/http"
	webhookapp "github.com/Mark-0731/SwiftMail/internal/features/webhook/application"
	webhookinfra "github.com/Mark-0731/SwiftMail/internal/features/webhook/infrastructure"
	webhookhttp "github.com/Mark-0731/SwiftMail/internal/features/webhook/transport/http"
	"github.com/Mark-0731/SwiftMail/internal/platform/cache"
	"github.com/Mark-0731/SwiftMail/internal/platform/queue"
	"github.com/Mark-0731/SwiftMail/internal/server/middleware"
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
	jwtManager := authdomain.NewJWTManager(cfg.JWT.AccessSecret, cfg.JWT.RefreshSecret, cfg.JWT.AccessExpiry, cfg.JWT.RefreshExpiry)
	apiKeyManager := authdomain.NewAPIKeyManager(rdb)
	totpManager := authdomain.NewTOTPManager()
	rateLimiter := ratelimit.NewTokenBucket(rdb)

	// ─── Repositories ────────────────────────────────────────────────────
	authRepo := authinfra.NewPostgresRepository(db)
	domainRepo := domainmgmtinfra.NewPostgresRepository(db)
	templateRepo := tmplinfra.NewPostgresRepository(db)
	emailRepo := emailinfra.NewPostgresEmailRepository(db)
	suppressionRepo := suppressioninfra.NewPostgresRepository(db)
	webhookRepo := webhookinfra.NewRepository(db)
	userRepo := userinfra.NewRepository(db)
	verificationRepo := verificationinfra.NewPostgresRepository(db)

	// ─── Infrastructure Adapters ─────────────────────────────────────────
	cacheAdapter := cache.NewRedisCache(rdb, logger)
	queueAdapter := queue.NewAsynqQueue(asynqClient, logger)

	// ─── Services ────────────────────────────────────────────────────────
	// Verification service (independent)
	verificationService := verificationapp.NewService(verificationRepo, queueAdapter, rdb, logger)

	// Auth service (depends on verification service)
	authService := authapp.NewService(authRepo, jwtManager, totpManager, apiKeyManager, rdb, logger, verificationService)
	dnsChecker := domainmgmtdomain.NewDNSChecker()
	domainService := domainmgmtapp.NewService(domainRepo, dnsChecker, rdb, logger)
	templateService := tmplapp.NewService(templateRepo, logger)

	// Billing services
	creditService := billingapp.NewCreditService(cacheAdapter, logger)
	stripeService := billinginfra.NewStripeService(
		cfg.Stripe.SecretKey,
		cfg.Stripe.WebhookSecret,
		cfg.Stripe.PublishableKey,
		cfg.Stripe.PriceIDStarter,
		cfg.Stripe.PriceIDPro,
		cfg.Stripe.SuccessURL,
		cfg.Stripe.CancelURL,
		logger,
	)
	billingService := billingapp.NewService(db, rdb, stripeService, logger)

	// Email orchestrator
	emailOrchestrator := emailapp.NewOrchestrator(emailRepo, templateService, cacheAdapter, queueAdapter, rdb, creditService, logger)

	suppressionService := suppressionapp.NewService(suppressionRepo, rdb, logger)
	webhookDispatcher := webhookapp.NewDispatcher(webhookRepo, logger)
	abuseDetector := abuseapp.NewDetector(db, rdb, logger)
	warmupScheduler := warmupapp.NewScheduler(db, logger)

	// Admin service with health checker
	healthChecker := admininfra.NewHealthChecker(db, nil, asynqClient)
	adminService := adminapp.NewAdminService(userRepo, healthChecker)

	// ─── Handlers ────────────────────────────────────────────────────────
	authHandler := authhttp.NewHandler(authService)
	domainHandler := domainmgmthttp.NewHandler(domainService)
	templateHandler := tmplhttp.NewHandler(templateService)
	emailHandler := emailhttp.NewHandler(emailOrchestrator, logger)
	suppressionHandler := suppressionhttp.NewHandler(suppressionService)
	trackingHandler := trackinghttp.NewHandler(asynqClient, rdb, logger)
	webhookHandler := webhookhttp.NewHandler(webhookRepo, webhookDispatcher)
	billingHandler := billinghttp.NewHandler(billingService)
	adminHandler := adminhttp.NewHandler(adminService)
	abuseHandler := abusehttp.NewHandler(abuseDetector)
	warmupHandler := warmuphttp.NewHandler(warmupScheduler)

	// ─── Public Routes (no auth) ─────────────────────────────────────────
	trackinghttp.RegisterRoutes(app, trackingHandler)

	// Stripe webhook (public, no auth)
	billinghttp.RegisterWebhookRoutes(app, billingHandler)

	// ─── API v1 ──────────────────────────────────────────────────────────
	v1 := app.Group("/v1")

	// Auth (public)
	authhttp.RegisterPublicRoutes(v1, authHandler)

	// ─── Authenticated Routes ────────────────────────────────────────────
	authenticated := v1.Group("", middleware.EitherAuth(jwtManager, apiKeyManager, rdb, logger))
	authenticated.Use(middleware.RateLimit(rateLimiter, cfg.RateLimit.PerSec, cfg.RateLimit.PerDay))

	// Auth (protected)
	authhttp.RegisterProtectedRoutes(authenticated, authHandler)

	// Domain routes
	domainmgmthttp.RegisterRoutes(authenticated, domainHandler)

	// Template routes
	tmplhttp.RegisterRoutes(authenticated, templateHandler)

	// Email send (with idempotency middleware)
	mailGroup := authenticated.Group("/mail")
	mailGroup.Use(middleware.Idempotency(rdb))
	mailGroup.Post("/send", emailHandler.Send)

	// Email log routes
	emailhttp.RegisterRoutes(authenticated, emailHandler)

	// Suppression routes
	suppressionhttp.RegisterRoutes(authenticated, suppressionHandler)

	// Webhook routes
	webhookhttp.RegisterRoutes(authenticated, webhookHandler)

	// Billing routes
	billinghttp.RegisterRoutes(authenticated, billingHandler)

	// ─── Admin (owner only) ──────────────────────────────────────────────
	adminGroup := v1.Group("", middleware.JWTAuth(jwtManager, logger), middleware.RequireRole("owner"))
	adminhttp.RegisterRoutes(adminGroup, adminHandler)
	abusehttp.RegisterRoutes(adminGroup, abuseHandler)
	warmuphttp.RegisterRoutes(adminGroup, warmupHandler)

	return app
}
