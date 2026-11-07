# Writer's Studio on Windows

This standalone Docker Compose bundle uses a release image supplied by your publisher.
It contains `compose.yaml`, `.env.example` and this guide. No official image registry
is configured by the template; obtain a versioned image reference before installing.

## Prepare the machine

Install Docker Desktop, start it with Linux containers enabled, and open PowerShell
in this directory. Verify that `docker version` and `docker compose version` work.
If the supplied registry requires authentication, use your own account and the access
method provided by its operator. Never distribute a publisher's account password.

## Configure the installation

```powershell
Copy-Item .env.example .env
notepad .env
```

Set `WRITERS_STUDIO_IMAGE` to the supplied versioned tag or immutable digest. Set
`POSTGRES_PASSWORD` and `OBJECT_STORAGE_SECRET_KEY` to different random URL-safe
values. A simple PowerShell command for a long hexadecimal value is:

```powershell
([guid]::NewGuid().ToString('N') + [guid]::NewGuid().ToString('N')).Substring(0,48)
```

Run it separately for each value. Leave provider keys empty to run without speech
recognition. For dictation, set your own `DEEPGRAM_API_KEY` and/or
`OPENROUTER_API_KEY`. The `.env` file remains local and must not be included in a
shared bundle or container image.

If users will save assistant-provider credentials in the application, generate a
32-byte base64 encryption key and set `LLM_CREDENTIAL_ENCRYPTION_KEY`. Keep a secure
backup of this key; changing it makes existing encrypted credentials unreadable.

```powershell
$bytes = New-Object byte[] 32
[System.Security.Cryptography.RandomNumberGenerator]::Create().GetBytes($bytes)
[Convert]::ToBase64String($bytes)
```

## Start and verify

```powershell
docker compose --env-file .env -f compose.yaml pull
docker compose --env-file .env -f compose.yaml up -d
docker compose --env-file .env -f compose.yaml ps
curl.exe -fsS http://localhost:8080/api/health
```

Open <http://localhost:8080> and create an application account. The app port is bound
to `127.0.0.1`, so this bundle does not expose the installation to other machines.
Browsers support microphone access on localhost without a separate HTTPS proxy.

## Troubleshooting

```powershell
docker compose --env-file .env -f compose.yaml logs --tail=150 app
docker compose --env-file .env -f compose.yaml logs --tail=150 db storage
```

An image pull failure means the reference is unavailable or registry authorization
needs attention. If port 8080 is occupied, set `APP_PORT=8081` in `.env`, run `up -d`
and use <http://localhost:8081>.

## Update or stop

Back up application data before updating. Change `WRITERS_STUDIO_IMAGE` to the desired
release, then repeat `pull` and `up -d`. To stop while retaining data:

```powershell
docker compose --env-file .env -f compose.yaml down
```

Named PostgreSQL and MinIO volumes survive ordinary stops and image updates.
`docker compose down -v` deletes those volumes and their manuscripts, audio and settings.
