# Contributing to Frame

Thanks for helping make Frame better. We welcome contributions of all sizes: docs, examples, tests, new integrations, and core features.

## What We’re Looking For

High-impact contributions that improve usability and adoption:

- Clear onboarding docs, tutorials, and recipes
- Example services that demonstrate real patterns
- New Go Cloud drivers (queue, cache, datastore)
- Middleware, interceptors, or plugins
- Testing utilities and benchmarks
- Performance or reliability improvements

## AI-Assisted Contributions

AI-assisted contributions are welcome. If you use AI tools, please:

- Verify behavior locally and include test coverage when relevant
- Avoid copy-pasting unverified output into production code
- Clearly describe what the change does and why
- Include any prompts or tool notes if they help reviewers understand the change

## Development Workflow

1. Fork the repo and create a branch.
2. Make changes with tests when possible.
3. Run:

```bash
go test -json -cover ./...
```

4. Open a pull request with a clear description, screenshots/logs if helpful.

## Style Expectations

- Keep APIs simple and composable
- Prefer explicit configuration via interfaces
- Avoid breaking changes unless clearly justified
- Keep docs up to date with code changes

## Reporting Issues

When filing issues, include:

- Frame version or commit hash
- Reproduction steps
- Logs or stack traces
- Environment details (Go version, OS)

## License

By contributing, you agree that your contributions will be licensed under the repository’s license.
