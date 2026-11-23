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

**Recommended: Cloudflare Tunnel + Access.** No inbound ports at all, SSO-style login for people and a
service token for machines, set up by one script. See [cloudflare-access.md](cloudflare-access.md).

Alternatives, if you do not use Cloudflare:

**SSH tunnel (simplest).** From your machine:

    ssh -L 3000:localhost:3000 -L 7401:localhost:7401 you@your-vps
    bordo setup --server http://localhost:7401

**Tailscale or WireGuard.** Install Tailscale on the VPS and your devices, then set
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

## The code sandbox

`code_exec` runs repo code in the `bordo-sandbox` container. It needs the internet for dependencies, but
its entrypoint installs an egress policy that rejects private, link-local, CGNAT and loopback
destinations: the Docker host and gateway, other containers, your LAN or VPC, and the cloud metadata
service at `169.254.169.254`. If the rules cannot be installed the container exits rather than run open.

To allow an internal package mirror, set `BORDO_SANDBOX_ALLOW_CIDRS="10.0.0.5/32"` in `.env`.
The container holds `NET_ADMIN` only so the entrypoint can do this; commands run as unprivileged
users under `no-new-privileges`. For stronger isolation of untrusted code, run Docker with gVisor
(`runsc`) or Sysbox as the runtime.
