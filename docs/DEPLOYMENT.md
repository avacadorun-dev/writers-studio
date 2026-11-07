# Deployment and operations

The root Compose file is for local development. It uses public example passwords,
internal database/storage networking, and a loopback web port. Do not expose this
configuration as a production service.

## LAN server

The production stack is `docker-compose.server.yml`. Set your own host and origin,
configure DNS, and use trusted HTTPS. `studio.example.test` in documentation is an
example, not a service operated by this project.

The environment initializer generates database/storage passwords and an MCP
consent secret. It expects a local trusted provider file containing a Deepgram
key and your chosen `PUBLIC_HOST` and `PUBLIC_BASE_URL`:

```sh
cp .env .env.providers
# Edit .env.providers for your deployment before continuing.
./deploy/init-server-env.sh
./deploy/update-server.sh
```

The initializer replaces `.env` and removes the provider input file after success.
Back up existing settings securely first. Environment files are shell input; do
not use files obtained from untrusted sources.

`deploy/Caddyfile` uses Caddy's internal CA. Install its public root certificate in
the client trust store when using that setup. Never commit a local CA/private key.
The proxy binds ports 80/443; the application itself stays on loopback. The optional
MCP gateway is intended for trusted LAN/VPN clients, not arbitrary internet access.

## Backups and updates

```sh
./deploy/backup-server.sh
./deploy/update-server.sh
```

The update script creates a database/object-storage backup before rebuilding and
waits for a health check. Keep copies off the host and test restoration.
PostgreSQL metadata, object storage and any credential-encryption key form one
recovery set. An application project export is not a complete server backup.

Production named volumes include `writers-studio-postgres-data` and
`writers-studio-object-data`. Ordinary container recreation retains them.
`docker compose down -v` or manual volume deletion destroys them.

## Data lifecycle

Audio retention is configurable for 30, 90 or 365 days. Applying retention removes
expired server audio, not unfinished local recovery chunks. Archive, trash and
permanent deletion are separate operations. Purge requires exact confirmation.
Account deletion removes owned data and schedules/removes associated objects;
external infrastructure backups follow the operator's retention policy.

Check `/api/health`, application logs and disk use without recording prompts,
credentials, audio or manuscripts in operational telemetry. This frozen repository
does not provide an operated hosting service or an availability commitment.
