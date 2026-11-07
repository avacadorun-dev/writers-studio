# Security policy

## Support status

This Go/web edition is frozen and provided as-is. No version has a guaranteed
security-maintenance window or response-time commitment. Self-hosters and fork
maintainers are responsible for monitoring and updating their dependencies and
infrastructure.

## Reporting a vulnerability

Do not publish working credentials, exploit data containing real user content,
or private deployment details in an issue. If the repository's GitHub Security
page offers **Report a vulnerability**, use that private channel. A dedicated
email address or private reporting service is not currently promised.

A useful report includes the affected version, a minimal reproduction using
synthetic data, the impact and suggested mitigations. If private reporting is
unavailable, an ordinary issue may request a private contact without including
sensitive details.

## Deployment responsibilities

- Keep `.env`, private keys, provider credentials, recordings, manuscripts,
  account backups and database dumps outside Git.
- Use unique deployment credentials. Default Compose passwords are development
  examples, not production secrets.
- Use trusted HTTPS for network microphone access. Restrict the optional remote
  MCP gateway to the intended LAN/VPN and issue narrowly scoped grants.
- Preserve the encryption key for stored user provider credentials securely;
  losing or changing it can make existing connections unreadable.
- Review provider processing/retention policies before sending audio or project
  content. This application cannot guarantee another provider's policies.
- Back up PostgreSQL and object storage together, and test restoration.

Before publishing a fork, run the release check and a full-history secret scanner.
A clean scan is evidence about tested patterns, not proof that no sensitive data
exists. If a real credential has been exposed, revoke or rotate it before
rewriting history. See [GitHub's removal guide](https://docs.github.com/en/authentication/keeping-your-account-and-data-secure/removing-sensitive-data-from-a-repository).
