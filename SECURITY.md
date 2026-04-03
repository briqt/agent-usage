# Security Policy

## Reporting a Vulnerability

If you discover a security vulnerability, please report it responsibly:

1. **Do NOT** open a public GitHub issue
2. Email the maintainer directly or use [GitHub Security Advisories](https://github.com/briqt/agent-usage/security/advisories/new)
3. Include steps to reproduce and potential impact

We will respond within 72 hours and work on a fix promptly.

## Scope

agent-usage runs locally and processes local files. Key security considerations:

- **Session data** may contain prompts, code snippets, and API usage details
- **SQLite database** stores aggregated usage data locally
- **Web dashboard** binds to a configurable port (default: 9800) — restrict access in shared environments
- **Pricing sync** makes outbound HTTPS requests to GitHub (litellm price data only)

## Best Practices

- The server binds to `127.0.0.1` by default (local-only). Set `bind_address: "0.0.0.0"` in config.yaml only if you need remote access
- Keep the SQLite database file permissions restricted
- Review `config.yaml` before sharing or committing
