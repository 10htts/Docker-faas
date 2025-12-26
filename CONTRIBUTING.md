# Contributing to Docker FaaS

Thank you for your interest in contributing to Docker FaaS! This document provides guidelines and instructions for contributing.

## Table of Contents

- [Code of Conduct](#code-of-conduct)
- [How to Contribute](#how-to-contribute)
- [Development Setup](#development-setup)
- [Coding Standards](#coding-standards)
- [Testing](#testing)
- [Pull Request Process](#pull-request-process)
- [Reporting Issues](#reporting-issues)

## Code of Conduct

### Our Pledge

We are committed to providing a welcoming and inclusive environment for all contributors.

### Our Standards

- Be respectful and professional
- Welcome constructive feedback
- Focus on what is best for the community
- Show empathy towards others

## How to Contribute

### Types of Contributions

We welcome:

1. **Bug Reports** - Help us identify and fix issues
2. **Feature Requests** - Suggest new features or improvements
3. **Code Contributions** - Submit pull requests for bugs or features
4. **Documentation** - Improve or add to documentation
5. **Testing** - Write tests or test new features
6. **Reviews** - Review pull requests from other contributors

### Getting Started

1. Fork the repository
2. Create a feature branch
3. Make your changes
4. Submit a pull request

## Development Setup

### Prerequisites

- Go 1.21 or later
- Docker 20.10+
- Docker Compose
- Make
- Git

### Setting Up Your Development Environment

```bash
# Clone your fork
git clone https://github.com/YOUR_USERNAME/docker-faas.git
cd docker-faas

# Add upstream remote
git remote add upstream https://github.com/docker-faas/docker-faas.git

# Install dependencies
make install-deps

# Build the project
make build

# Run tests
make test
```

### Running Locally

```bash
# Start the gateway
go run ./cmd/gateway

# Or use docker-compose
docker-compose up
```

## Coding Standards

### Go Style Guide

Follow the official [Go Code Review Comments](https://github.com/golang/go/wiki/CodeReviewComments).

### Key Principles

1. **Clarity over Cleverness** - Write clear, readable code
2. **DRY** - Don't Repeat Yourself
3. **KISS** - Keep It Simple, Stupid
4. **Error Handling** - Always handle errors appropriately
5. **Comments** - Comment on "why", not "what"

### Code Formatting

```bash
# Format code
make fmt

# Run linter
make lint

# Run vet
make vet
```

### Project Structure

```
pkg/
├── config/      # Configuration management
├── gateway/     # HTTP handlers
├── middleware/  # HTTP middleware
├── metrics/     # Prometheus metrics
├── provider/    # Docker provider
├── router/      # Request routing
├── store/       # Database layer
└── types/       # Type definitions
```

### Naming Conventions

- **Packages**: lowercase, single word
- **Files**: lowercase with underscores
- **Types**: PascalCase
- **Functions**: PascalCase (exported) or camelCase (private)
- **Variables**: camelCase
- **Constants**: PascalCase or SCREAMING_SNAKE_CASE

### Examples

Good:
```go
// GetFunction retrieves a function by name
func (s *Store) GetFunction(name string) (*types.FunctionMetadata, error) {
    if name == "" {
        return nil, fmt.Errorf("function name is required")
    }
    // ...
}
```

Bad:
```go
func (s *Store) get_function(n string) (*types.FunctionMetadata, error) {
    // ...
}
```

## Testing

### Writing Tests

- Write unit tests for all new code
- Maintain or improve code coverage
- Use table-driven tests where appropriate
- Mock external dependencies

### Test Structure

```go
func TestFunctionName(t *testing.T) {
    t.Run("DescriptiveTestName", func(t *testing.T) {
        // Arrange
        input := "test"

        // Act
        result, err := FunctionUnderTest(input)

        // Assert
        assert.NoError(t, err)
        assert.Equal(t, expected, result)
    })
}
```

### Running Tests

```bash
# Run all tests
make test

# Run with coverage
make coverage

# Run specific package
go test ./pkg/store/...

# Run integration tests
make integration-test
```

### Test Coverage

- Aim for >80% code coverage
- Critical paths should have 100% coverage
- View coverage report: `make coverage`

## Pull Request Process

### Before Submitting

1. **Update your fork**:
   ```bash
   git fetch upstream
   git rebase upstream/main
   ```

2. **Create a feature branch**:
   ```bash
   git checkout -b feature/your-feature-name
   ```

3. **Make your changes**:
   - Write code following style guidelines
   - Add tests for new functionality
   - Update documentation if needed

4. **Run checks**:
   ```bash
   make fmt
   make vet
   make test
   ```

5. **Commit your changes**:
   ```bash
   git add .
   git commit -m "Add feature: description"
   ```

### Commit Message Format

Follow the [Conventional Commits](https://www.conventionalcommits.org/) specification:

```
<type>(<scope>): <subject>

<body>

<footer>
```

Types:
- `feat`: New feature
- `fix`: Bug fix
- `docs`: Documentation changes
- `test`: Test additions or changes
- `refactor`: Code refactoring
- `perf`: Performance improvements
- `chore`: Build process or auxiliary tool changes

Examples:
```
feat(gateway): add support for async invocations

Add new endpoint for async function invocations using queue.
Functions can now be invoked asynchronously with callback URLs.

Closes #123
```

```
fix(provider): handle container cleanup on scale down

Previously, scaling down would leave orphaned containers.
Now properly removes excess containers when scaling.

Fixes #456
```

### Submitting the PR

1. Push to your fork:
   ```bash
   git push origin feature/your-feature-name
   ```

2. Open a Pull Request on GitHub

3. Fill out the PR template completely

4. Link related issues

5. Wait for review

### PR Review Process

1. **Automated Checks**: CI must pass
2. **Code Review**: At least one maintainer approval required
3. **Testing**: Verify tests pass locally and in CI
4. **Documentation**: Ensure docs are updated
5. **Merge**: Maintainers will merge when approved

### Addressing Review Comments

```bash
# Make changes based on feedback
git add .
git commit -m "Address review comments"
git push origin feature/your-feature-name
```

## Reporting Issues

### Bug Reports

Use the bug report template and include:

1. **Description**: Clear description of the bug
2. **Steps to Reproduce**: Detailed steps
3. **Expected Behavior**: What should happen
4. **Actual Behavior**: What actually happens
5. **Environment**: OS, Docker version, etc.
6. **Logs**: Relevant log output

### Feature Requests

Use the feature request template and include:

1. **Problem**: What problem does this solve?
2. **Solution**: Proposed solution
3. **Alternatives**: Alternative solutions considered
4. **Additional Context**: Any other relevant info

### Security Issues

**Do NOT open public issues for security vulnerabilities.**

Email security@docker-faas.io with:
- Description of the vulnerability
- Steps to reproduce
- Potential impact
- Suggested fix (if any)

## Development Workflow

### Branching Strategy

- `main`: Stable, production-ready code
- `develop`: Integration branch for features
- `feature/*`: New features
- `bugfix/*`: Bug fixes
- `hotfix/*`: Urgent production fixes

### Release Process

1. Create release branch from `develop`
2. Update version numbers
3. Update CHANGELOG
4. Test thoroughly
5. Merge to `main` and tag
6. Merge back to `develop`

## Additional Resources

- [Go Documentation](https://golang.org/doc/)
- [Docker Engine API](https://docs.docker.com/engine/api/)
- [OpenFaaS](https://docs.openfaas.com/)
- [Prometheus](https://prometheus.io/docs/)

## Questions?

- Open a [Discussion](https://github.com/docker-faas/docker-faas/discussions)
- Join our community chat (coming soon)
- Check existing [Issues](https://github.com/docker-faas/docker-faas/issues)

## License

By contributing, you agree that your contributions will be licensed under the MIT License.

Thank you for contributing to Docker FaaS! 🎉
