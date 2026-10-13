# Security Policy

> **No security maintenance (2026-10-13):** This project is no longer maintained and the repository is archived. No versions are supported, and no further security fixes or dependency updates will be provided. Existing binaries and container images remain available without ongoing security maintenance. See the [README](README.md) for the recommended alternative.
>
> **停止安全维护（2026-10-13）：** 本项目已停止维护并归档，所有版本均不再获得安全修复或依赖更新。现有二进制和容器镜像仅作为历史产物保留，不再承诺漏洞响应或修复。替代工具建议见 [中文 README](README_CN.md)。

## Reporting a Vulnerability

If you discover a security vulnerability, please report it responsibly:

1. **Do NOT** open a public GitHub issue
2. Email the maintainer directly or use [GitHub Security Advisories](https://github.com/briqt/agent-usage/security/advisories/new)
3. Include steps to reproduce and potential impact

Because this project is no longer maintained, vulnerability reports may not receive a response or a fix. There is no response-time commitment. Please avoid disclosing sensitive session data or credentials in any report.

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
