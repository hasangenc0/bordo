# Securing a Bordo VPS

Bordo controls code execution (the agent's sandbox), your GitHub token and your fleet, so treat the
VPS as a high-value target. A stolen password alone should not be enough to reach it.

## What the stack exposes

`docker compose` publishes three ports. They bind to **127.0.0.1** by default:

| Port | Service | Who needs it |
|---|---|---|
| 3000 | Web console (basic auth) | you |
| 7401 | bordod API | the CLI, and remote regions (reconciler / node agent) |
| 7402 | Agent | the console (inside Docker); the CLI's `agent chat` |

> **Docker bypasses ufw.** Docker publishes ports with its own iptables rules, so `ufw deny 7402`
> does not close a port bound to `0.0.0.0`. The bind address is the control that matters.

## Reaching it

Pick one.

**SSH tunnel (simplest).** From your machine:

    ssh -L 3000:localhost:3000 -L 7401:localhost:7401 you@your-vps
    bordo setup --server http://localhost:7401

**Tailscale (recommended for daily use).** Install Tailscale on the VPS and your devices, then set
`BIND_ADDR=<the VPS's tailscale IP>` in `.env`. Nothing is reachable from the public internet.

**HTTPS on a domain (Caddy profile).** Point `<domain>` and `api.<domain>` at the VPS, then in `.env`:

    BORDO_DOMAIN=bordo.example.com

and run `docker compose --profile tls up -d`. Caddy obtains certificates, serves the console at
`https://<domain>` and bordod at `https://api.<domain>`. Keep the loopback bind so only 80/443 are
public. Set the base URL with `bordo config set base_url https://api.<domain>`.

Remote regions pull from bordod, so it must be reachable from them: use the `api.` host above or a
Tailscale IP, not a tunnel.

## Host checklist

- SSH: key-only (`PasswordAuthentication no`), `PermitRootLogin no`, a non-default user.
- `ufw default deny incoming`, allow only 22 (and 80/443 if you use the TLS profile).
- `unattended-upgrades` for security patches.
- CrowdSec or fail2ban for SSH.
- Keep `deploy/bordo/.env` at mode 600.
- If you ever ran an older version with port 7401 reachable from the internet, rotate the GitHub
  login and the console password: `GET /github/token` used to answer without authentication.
