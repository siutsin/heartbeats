# Contributing to Heartbeats Operator

Thank you for your interest in contributing to the Heartbeats Operator! This document
provides guidelines and instructions for contributing to the project.

## Development Environment Setup

1. Fork the repository

2. Clone your fork:

    ```bash
    git clone https://github.com/your-username/heartbeats.git
    cd heartbeats
    ```

3. Install dependencies:

    ```bash
    make install
    ```

## Development Workflow

1. Create a new branch for your feature or bugfix:

    ```bash
    git checkout -b feature/your-feature-name
    ```

2. Make your changes

3. Run tests:

    ```bash
    make test
    make test-e2e
    ```

4. Commit your changes with a descriptive message

5. Push your branch to your fork

6. Create a pull request

## Code Style

- Follow the Go code style guidelines
- Run `make lint` to check for style issues
- Ensure all tests pass before submitting a pull request

## Pull Request Process

1. Ensure your PR description clearly describes the problem and solution

2. Include relevant tests

3. Update documentation if necessary

4. The PR must pass all CI checks

5. A maintainer will review your PR and may request changes

## Reporting Issues

When reporting issues, please include:

- Description of the problem
- Steps to reproduce
- Expected behaviour
- Actual behaviour
- Environment details (Kubernetes version, etc.)

## License

By contributing, you agree that your contributions will be licensed under the Apache License 2.0.
