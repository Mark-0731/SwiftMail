# Contributing to SwiftMail

Thank you for your interest in contributing to SwiftMail! This document provides guidelines for contributing to the project.

## Code of Conduct

By participating in this project, you agree to maintain a respectful and inclusive environment for all contributors.

## How to Contribute

### Reporting Bugs

1. Check if the bug has already been reported in [Issues](https://github.com/Mark-0731/SwiftMail-BE/issues)
2. If not, create a new issue with:
   - Clear title and description
   - Steps to reproduce
   - Expected vs actual behavior
   - Environment details (OS, Go version, etc.)

### Suggesting Features

1. Check existing feature requests
2. Create a new issue with the `enhancement` label
3. Describe the feature and its use case
4. Explain why it would be valuable

### Pull Requests

1. **Fork the repository**
2. **Create a feature branch**
   ```bash
   git checkout -b feature/your-feature-name
   ```

3. **Make your changes**
   - Follow Go best practices
   - Write clear, commented code
   - Add tests for new features
   - Update documentation

4. **Test your changes**
   ```bash
   go test ./...
   go fmt ./...
   go vet ./...
   ```

5. **Commit with clear messages**
   ```bash
   git commit -m "feat: add email template validation"
   ```

6. **Push and create PR**
   ```bash
   git push origin feature/your-feature-name
   ```

## Development Setup

1. Install Go 1.21+
2. Install PostgreSQL and Redis
3. Copy `.env.example` to `.env`
4. Run migrations: `go run cmd/migrate/main.go`
5. Start services: `go run cmd/api/main.go`

## Coding Standards

- Follow [Effective Go](https://golang.org/doc/effective_go)
- Use `gofmt` for formatting
- Write meaningful variable names
- Add comments for complex logic
- Keep functions small and focused

## Commit Message Format

```
type: subject

body (optional)

footer (optional)
```

Types:
- `feat`: New feature
- `fix`: Bug fix
- `docs`: Documentation
- `style`: Formatting
- `refactor`: Code restructuring
- `test`: Adding tests
- `chore`: Maintenance

## Testing

- Write unit tests for new features
- Maintain test coverage above 70%
- Test edge cases and error handling

## Questions?

Feel free to ask questions in:
- GitHub Issues
- Discord community
- Email: dev@swiftmail.com

Thank you for contributing! 🚀
