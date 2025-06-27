# Contributing to HyperServe

Thank you for your interest in contributing to HyperServe! We welcome contributions from the community.

## Getting Started

1. Fork the repository on GitHub
2. Clone your fork locally:
   ```bash
   git clone https://github.com/YOUR_USERNAME/hyperserve.git
   cd hyperserve
   ```
3. Create a branch for your changes:
   ```bash
   git checkout -b feature/your-feature-name
   ```

## Development Guidelines

### Code Style

- Follow standard Go conventions and idioms
- Run `go fmt ./...` before committing
- Run `go vet ./...` to catch common mistakes
- Keep functions small and focused
- Add comments for exported functions and types

### Testing

- Write tests for new functionality
- Ensure all tests pass: `go test ./...`
- Run tests with race detection: `go test -race ./...`
- Aim for good test coverage

### Commit Messages

- Use clear, descriptive commit messages
- Start with a verb in present tense (e.g., "Add", "Fix", "Update")
- Keep the first line under 50 characters
- Add detailed description if needed

Example:
```
Add rate limiting middleware

- Implement token bucket algorithm
- Add per-IP rate limiting
- Include configurable burst capacity
```

## Submitting Changes

1. Ensure all tests pass
2. Update documentation if needed
3. Push your changes to your fork
4. Create a Pull Request with:
   - Clear description of changes
   - Any related issue numbers
   - Screenshots if applicable

## Reporting Issues

- Use GitHub Issues to report bugs
- Include Go version and OS
- Provide minimal reproduction steps
- Include error messages and stack traces

## Code of Conduct

- Be respectful and inclusive
- Welcome newcomers and help them get started
- Focus on constructive criticism
- Assume good intentions

## Questions?

Feel free to open an issue for any questions about contributing.

Thank you for helping make HyperServe better!