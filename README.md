# SwiftMail Backend

⚡ Fast, reliable, and scalable email delivery platform built with Go.

## 🚀 Features

- **High-Performance Email Delivery** - Send thousands of emails per second
- **Real-Time Analytics** - Track opens, clicks, deliveries, and bounces
- **Multi-Provider Support** - SMTP, SendGrid, AWS SES, and more
- **Advanced Queue Management** - Redis-based job queue with priority handling
- **Domain Management** - SPF, DKIM, and DMARC configuration
- **Template Engine** - Dynamic email templates with variable substitution
- **Suppression Lists** - Automatic bounce and complaint handling
- **Webhook Support** - Real-time event notifications
- **API Key Management** - Secure authentication with rate limiting
- **Credit System** - Top-up based billing model

## 📋 Prerequisites

- Go 1.21 or higher
- PostgreSQL 14+
- Redis 7+
- SendGrid account (or other SMTP provider)

## 🛠️ Installation

1. **Clone the repository**
```bash
git clone https://github.com/Mark-0731/SwiftMail-BE.git
cd SwiftMail-BE
```

2. **Install dependencies**
```bash
go mod download
```

3. **Configure environment variables**
```bash
cp .env.example .env
# Edit .env with your configuration
```

4. **Run database migrations**
```bash
go run cmd/migrate/main.go
```

5. **Start the API server**
```bash
go run cmd/api/main.go
```

6. **Start the worker (in another terminal)**
```bash
go run cmd/worker/main.go
```

## 🔧 Configuration

Key environment variables:

```env
# Server
SERVER_PORT=8080
APP_ENV=production

# Database
DB_HOST=localhost
DB_PORT=5432
DB_USER=postgres
DB_PASSWORD=your_password
DB_NAME=swiftmail

# Redis
REDIS_HOST=localhost
REDIS_PORT=6379

# SMTP
SMTP_HOST=smtp.sendgrid.net
SMTP_PORT=587
SMTP_USERNAME=apikey
SMTP_PASSWORD=your_sendgrid_api_key

# JWT
JWT_ACCESS_SECRET=your_secret_key
JWT_REFRESH_SECRET=your_refresh_secret
```

## 📚 API Documentation

### Authentication

All API requests require an API key in the header:

```bash
X-API-Key: sm_live_your_api_key
```

### Send Email

```bash
POST /v1/mail/send
Content-Type: application/json

{
  "from": "sender@example.com",
  "to": "recipient@example.com",
  "subject": "Hello World",
  "html": "<h1>Hello!</h1>",
  "text": "Hello!"
}
```

### Get Email Logs

```bash
GET /v1/mail/logs?page=1&limit=50
```

### Track Analytics

```bash
GET /v1/analytics/summary?start_date=2024-01-01&end_date=2024-01-31
```

## 🏗️ Architecture

```
SwiftMail-BE/
├── cmd/
│   ├── api/          # API server entry point
│   ├── worker/       # Background worker entry point
│   ├── migrate/      # Database migrations
│   └── smtp/         # SMTP server (optional)
├── internal/
│   ├── config/       # Configuration management
│   ├── features/     # Feature modules (DDD structure)
│   │   ├── auth/     # Authentication & authorization
│   │   ├── email/    # Email sending logic
│   │   ├── analytics/# Analytics tracking
│   │   ├── billing/  # Credit system
│   │   └── ...
│   ├── platform/     # Infrastructure layer
│   │   ├── queue/    # Job queue (Asynq)
│   │   ├── cache/    # Redis cache
│   │   └── provider/ # Email providers
│   └── server/       # HTTP server setup
├── pkg/              # Shared packages
│   ├── database/     # Database utilities
│   ├── metrics/      # Prometheus metrics
│   └── tracking/     # Email tracking
└── migrations/       # SQL migrations
```

## 🧪 Testing

Run tests:
```bash
go test ./...
```

Run tests with coverage:
```bash
go test -cover ./...
```

## 📊 Monitoring

SwiftMail exposes Prometheus metrics at:
- API: `http://localhost:9091/metrics`
- Worker: `http://localhost:9092/metrics`

Key metrics:
- `swiftmail_emails_sent_total` - Total emails sent
- `swiftmail_emails_failed_total` - Total failed emails
- `swiftmail_email_send_duration_seconds` - Email send latency

## 🔒 Security

- API keys are hashed using bcrypt
- JWT tokens for session management
- Rate limiting on all endpoints
- SQL injection prevention with parameterized queries
- CORS protection
- Request ID tracking for audit logs

## 🤝 Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for contribution guidelines.

## 📄 License

This project is licensed under the MIT License - see [LICENSE](LICENSE) file for details.

## 🐛 Bug Reports

Please report bugs via [GitHub Issues](https://github.com/Mark-0731/SwiftMail-BE/issues).

## 📧 Support

For support, email support@swiftmail.com or join our [Discord community](https://discord.gg/swiftmail).

## 🙏 Acknowledgments

- [Fiber](https://gofiber.io/) - Web framework
- [Asynq](https://github.com/hibiken/asynq) - Job queue
- [GORM](https://gorm.io/) - ORM library
- [SendGrid](https://sendgrid.com/) - Email delivery

---

Made with ❤️ by the SwiftMail Team
