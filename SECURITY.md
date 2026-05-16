# Security Policy

MarkGo is a Go-based blog engine designed for self-hosted use. The threat
model is a single operator running the binary on their own infrastructure;
readers consume static HTML over HTTPS. There is no multi-tenancy, no user
accounts beyond the operator, and no persistent storage of user-submitted
data except AMA questions, which the operator moderates before publishing.

## Supported Versions

Only the latest minor release receives security fixes.

| Version | Supported          |
| ------- | ------------------ |
| 3.8.x   | Yes                |
| <= 3.7.x | No (upgrade required) |

## Reporting a Vulnerability

Report security issues **privately** via one of:

1. **Preferred:** [GitHub Security Advisory](https://github.com/1mb-dev/markgo/security/advisories/new)
2. **Email fallback:** security@1mb.dev

Do **not** open a public GitHub issue for unpatched vulnerabilities.

## Expected Response

- Acknowledgment within **7 days** of report
- Initial assessment (severity, scope) within **14 days**
- Coordinated disclosure timeline agreed with reporter
- Public advisory + patched release **at most 90 days** after report,
  sooner if the fix is straightforward

## Scope

**In scope** — will be triaged and fixed:

- Authentication / authorization bypass
- Remote code execution
- Cross-site scripting, CSRF, injection vulnerabilities
- Information disclosure (leaking drafts, configuration, internals)
- Denial of service against the markgo binary itself

**Out of scope** — acknowledged but not treated as vulnerabilities:

- DoS achievable only via overwhelming a single-instance deployment
  (mitigation is the operator's reverse proxy / rate-limit at network edge)
- Vulnerabilities in third-party dependencies (we accept Dependabot
  patches; report upstream)
- Issues requiring physical access or operator credentials

## Disclosure History

| Advisory | Affected | Fixed in | Date       |
| -------- | -------- | -------- | ---------- |
| [Admin JSON auth bypass + /metrics exposure](CHANGELOG.md) | <= v3.7.0 | v3.8.0 | 2026-05-XX |
