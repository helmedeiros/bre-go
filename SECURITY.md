# Security Policy

## Supported versions

The project has not yet cut a stable release. While in pre-1.0, only `main` receives security fixes; tagged pre-releases are kept for reference only.

## Reporting a vulnerability

Please **do not** open a public GitHub issue for security problems.

Instead, use GitHub's private vulnerability reporting:

> <https://github.com/helmedeiros/bre-go/security/advisories/new>

Include:

- a clear description of the issue,
- steps to reproduce,
- the version or commit you tested against,
- any proposed remediation if you have one.

You should receive an acknowledgement within seven days. If the issue is confirmed, a fix and a release timeline will be communicated back to you before any public disclosure.

## Scope

bre-go is a library that executes business rules. Reports in scope include:

- Unsafe deserialisation paths.
- Code execution via rule inputs or rule definitions.
- Privilege-escalation in any of the engine adapters.
- Resource exhaustion that a small input can trigger (DoS).

Out of scope: bugs in the rule definitions a downstream user writes; misconfigurations of the operator's own runtime.
