# Publishing Bordo with Cloudflare Tunnel + Access

The VPS opens **no inbound ports**. A `cloudflared` container connects out to Cloudflare, which serves
two hostnames and decides who may reach them:

| Host | Who gets in | How |
|---|---|---|
| `bordo.example.com` (console) | people on your email list | Access, built-in **One-time PIN**: enter your email, Cloudflare mails a code. No identity provider or OAuth app to set up. |
| `bordo-api.example.com` (bordod) | machines only: the CLI, `bordo-node`, the on-cluster reconciler | Access **service token**, sent as `CF-Access-Client-Id` / `CF-Access-Client-Secret`. |

Cloudflare rejects a request before it reaches your server unless it passes Access. The Bordo bearer
tokens and console password stay in place behind it as a second layer.

## One-time setup (all through the API)

1. Add your domain to Cloudflare, and create an API token at dash.cloudflare.com/profile/api-tokens with:
   Account: *Cloudflare Tunnel: Edit*, *Access: Apps and Policies: Edit*, *Access: Service Tokens: Edit*,
   *Account Settings: Read*, and Zone: *DNS: Edit*.
2. Run the script on the VPS (needs `curl` and `jq`):

       export CF_API_TOKEN=...
       export BORDO_CONSOLE_HOST=bordo.example.com
       export BORDO_API_HOST=bordo-api.example.com
       export BORDO_ACCESS_EMAILS=you@example.com,teammate@example.com
       deploy/cloudflare/setup.sh

   It creates the tunnel, its routes, the DNS records, the service token, two Access policies and two
   Access apps, then writes `CLOUDFLARE_TUNNEL_TOKEN` and the service token into `deploy/bordo/.env`
   (mode 600). Re-running is safe; `--dry-run` shows what it would do, `--rotate-token` issues a new
   machine secret. The API token is only used by the script.
3. Start the tunnel: `docker compose --profile cloudflare up -d`.
4. Tell Bordo its public API address so join commands use it: `bordo config set base_url https://bordo-api.example.com`.
5. Close ports 80 and 443 (and anything but SSH) in your firewall and the provider's cloud firewall.

## Using it

**Console:** open `https://bordo.example.com`, enter an allowed email, enter the code.

**CLI:**

    export BORDO_CF_ACCESS_CLIENT_ID=<client id>
    export BORDO_CF_ACCESS_CLIENT_SECRET=<client secret>      # from deploy/bordo/.env
    bordo config set server https://bordo-api.example.com

(or store them with `bordo config set cf_access_client_id ...` / `cf_access_client_secret ...`). The CLI
sends the token only to that https host.

**Nodes** (new regions): run the install command with the token and the API URL:

    curl -fsSL .../install-node.sh | \
      BORDO_CP_URL=https://bordo-api.example.com \
      BORDO_CF_ACCESS_CLIENT_ID=... BORDO_CF_ACCESS_CLIENT_SECRET=... \
      BORDO_JOIN_TOKEN=... sh

The node agent stores the token in a Secret next to the reconciler's cluster token.

**Existing regions** keep the old `BORDO_CP_URL` baked into their reconciler. To move one, update
the reconciler's Secret and URL in that cluster:

    kubectl -n bordo-system patch secret bordo-reconciler-creds --type merge -p \
      '{"stringData":{"cf-access-client-id":"...","cf-access-client-secret":"..."}}'
    kubectl -n bordo-system set env deploy/bordo-reconciler BORDO_CP_URL=https://bordo-api.example.com
    kubectl -n bordo-system rollout restart deploy/bordo-reconciler

Do this before you remove the old public address, or the region stops reconciling.

## Rotation and expiry

The machine token lasts one year by default (`CF_TOKEN_DURATION`). Cloudflare sends expiry notifications to the account.
Rotate with `deploy/cloudflare/setup.sh --rotate-token`, then update the CLI and nodes. Rotating invalidates
the old secret immediately, so update the regions' Secrets first or in the same sitting.

## Notes

- The iOS app reaches the agent through its own SSH tunnel and is not affected.
- Identity providers (Google, GitHub, Okta) can be added in Cloudflare's Zero Trust settings later; the
  policies the script creates keep working, and you can switch the console policy to require one.
- Only `bordo-node` and the reconciler need the machine token on remote hosts. Treat it like a password:
  anyone holding it can reach the API host (they still need a valid Bordo bearer token to do anything).
