# Security Policy

## Supported Versions

| Version | Supported          |
| ------- | ------------------ |
| 1.x.x   | :white_check_mark: |
| < 1.0   | :x:                |

## Reporting a Vulnerability

We take security seriously. If you discover a security vulnerability, please follow these steps:

### 1. Do NOT create a public GitHub issue

Security vulnerabilities should be reported privately to protect users.

### 2. Email us directly

Send details to: **security@swiftmail.com**

Include:
- Description of the vulnerability
- Steps to reproduce
- Potential impact
- Suggested fix (if any)

### 3. Response Timeline

- **24 hours**: Initial acknowledgment
- **7 days**: Detailed response with assessment
- **30 days**: Fix deployed (for confirmed vulnerabilities)

### 4. Disclosure Policy

- We will work with you to understand and fix the issue
- We request you keep the vulnerability confidential until we release a fix
- We will credit you in the security advisory (unless you prefer to remain anonymous)

## Security Best Practices

When using SwiftMail:

1. **API Keys**
   - Never commit API keys to version control
   - Rotate keys regularly
   - Use environment variables

2. **Database**
   - Use strong passwords
   - Enable SSL connections
   - Restrict network access

3. **SMTP**
   - Use TLS/SSL for connections
   - Verify sender domains
   - Implement rate limiting

4. **Authentication**
   - Use strong JWT secrets
   - Implement token expiration
   - Enable 2FA for admin accounts

## Known Security Features

- API key hashing with bcrypt
- JWT token-based authentication
- Rate limiting on all endpoints
- SQL injection prevention
- CORS protection
- Request ID tracking for audit logs

## Security Updates

Subscribe to security advisories:
- GitHub Security Advisories
- Email: security-updates@swiftmail.com

Thank you for helping keep SwiftMail secure! 🔒
