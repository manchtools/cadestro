# Contributing to Cadestro

## While the repository is being assembled

Cadestro is being consolidated from its predecessor repositories. Until the
seed completes, module content, build commands, and CI arrive incrementally —
this guide grows with them. What is stable already:

## Workflow

1. Create a branch from `main`.
2. Make your changes with conventional commit messages:
   - `feat:` new feature
   - `fix:` bug fix
   - `chore:` maintenance
   - `docs:` documentation
   - `refactor:` code restructuring
   - `perf:` performance improvement
   - `test:` test additions/changes
3. Open a pull request. Automated review runs on every PR.
4. Ensure CI passes before requesting review.

## Ground rules

- Every factual claim in documentation must be derivable from the code in this
  repository.
- Bug fixes come with a regression test that fails on the buggy version.
- Never edit generated code by hand; regenerate through the pinned tooling.
- Each Go module builds standalone (`GOWORK=off`); CI enforces it, because a
  module that only compiles inside the workspace has an undeclared dependency.

## Licensing of contributions

Each module carries its own license — see [LICENSING.md](LICENSING.md). By
contributing, you agree that your contributions are licensed under the license
of the module they touch.

## Security

Do not report vulnerabilities through public issues — see
[SECURITY.md](SECURITY.md).
