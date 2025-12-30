# Agent Instructions

This document provides directives for LLM agents working on this project.

## Core Principles

### Language and Format

- **British English Only**: Write all comments, documentation, and commit messages in British English (e.g., "colour", "behaviour"). Format dates as ISO 8601 (YYYY-MM-DD).
- **No Emojis**: Never use emojis in code, comments, documentation, or commit messages. Keep communication professional.

### Error Handling Policy

- **Fix, Never Suppress**: Always fix the root cause of warnings and errors. Never suppress or hide them (e.g., no output filtering or disabling lint rules).

### Automation Behaviour

- **Auto-Fix**: Automatically apply formatting fixes and run tests without asking.
- **When to Ask**: Only ask for confirmation for significant refactoring, behaviour changes, or architectural decisions.

### Project Conventions

- **Consistency**: Follow established patterns and match the style of surrounding code.

## Development Standards

### Code Standards

- **Editor Configuration**: **CRITICAL**: Always follow `.editorconfig` settings.

### Go Best Practices

- **Standards**: Follow the [Google Go Style Guide](https://google.github.io/styleguide/go/guide.html) as the primary coding style.
- **Reference**: Use [Go by Example](https://gobyexample.com/) for idiomatic patterns and [Go Documentation](https://go.dev/doc/) for the latest language features.
- **Project Layout**: Adhere to the [Standard Go Project Layout](https://github.com/golang-standards/project-layout).
- **Error Messages**: Declare as pre-formatted variables and reuse across tests; avoid hardcoded strings.
- **File Structure**: Split code into small files, each doing one specific job.
- **Pointers vs Values**: Prioritise interfaces and concrete values. Use pointers only when modifying the original object.
- **Concurrency**: Use channels for orchestration, mutexes for state. Avoid goroutine leaks by ensuring termination.
- **Interfaces**: Define interfaces where they are used (consumer-defined). Keep them small.

### Kubernetes Operator Standards

- **Design**: Create idiomatic Go code leveraging K8s patterns (CRDs, finalizers). Design for declarative configuration and eventual consistency.
- **Best Practices**:
  - Use exponential backoff for retries.
  - Use informers and work queues efficiently.
  - Implement proper RBAC and health checks.
  - Use structured logging with appropriate verbosity.
- **Performance**: Minimise API load (watch predicates), use caching/indexing, and implement rate limiting.
- **Production Readiness**: Handle graceful shutdown/leader election, expose Prometheus metrics, and plan for upgrades.

### Documentation Requirements

- Add docstrings to all new functions/classes and update them when signatures change.

### Markdown Guidelines

- Follow strict heading hierarchy (H1 → H2 → H3) and check against `markdownlint`.

## Testing & Verification

### Testing Protocol

- **Linting**: Ensure code passes `go fmt`, `go vet`, and `golangci-lint`.
- **Verification**: Run `make lint-markdown-fix && make lint && make test && make test-e2e LOCAL=true`.
- **Requirement**: Verify all checks pass before completion.

### Unit Test Standards

- **Timeouts**: Must be < 10 seconds.
- **Approach**: Use table testing and blackbox testing (`_test` suffix).
- **Mocks**: Use mocks to avoid actual network requests.

### Integration & E2E Testing

- **Ephemeral Stack**: Launch an ephemeral stack to verify the application behaves correctly in a production-like environment.

## Tool Selection

Use the appropriate tool for each file type:

| File Type | Tool           | Purpose                   |
| --------- | -------------- | ------------------------- |
| YAML      | `yq`           | Parsing and modification  |
| Markdown  | `markdownlint` | Linting and validation    |
| Go        | `go`           | Language toolchain        |
| Makefile  | `make`         | Build and test automation |
