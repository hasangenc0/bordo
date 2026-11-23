#!/usr/bin/env bash
# Put Bordo behind Cloudflare Tunnel + Access, entirely through the Cloudflare API.
#
# What it creates (idempotent: re-running updates what exists):
#   - a Tunnel the 'cloudflared' compose service connects out to (the VPS opens no inbound ports)
#   - two proxied DNS records: the console host and the API host
#   - an Access service token for machines (CLI, node agent, reconciler)
#   - two reusable Access policies and two Access apps:
#       console host -> people: allow the listed emails (built-in One-time PIN, no IdP needed)
#       API host     -> machines only: allow the service token, nobody else
#
# Needs: bash, curl, jq, and an API token with these permissions:
#   Account: Cloudflare Tunnel: Edit | Account: Access: Apps and Policies: Edit
#   Account: Access: Service Tokens: Edit | Zone: DNS: Edit | Account Settings: Read
#
# Usage:
#   export CF_API_TOKEN=...                      # create at dash.cloudflare.com/profile/api-tokens
#   export BORDO_CONSOLE_HOST=bordo.example.com
#   export BORDO_API_HOST=bordo-api.example.com
#   export BORDO_ACCESS_EMAILS=you@example.com,teammate@example.com
#   deploy/cloudflare/setup.sh
#
# Optional: CF_ACCOUNT_ID, CF_ZONE_ID (auto-detected when unambiguous), BORDO_TUNNEL_NAME (default
# bordo), BORDO_ENV_FILE (default deploy/bordo/.env), CF_SESSION_DURATION (default 24h),
# CF_TOKEN_DURATION (service token lifetime, default 8760h = 1 year), --rotate-token, --dry-run.
#
# Writes CLOUDFLARE_TUNNEL_TOKEN, BORDO_CF_ACCESS_CLIENT_ID and BORDO_CF_ACCESS_CLIENT_SECRET to the env
# file (mode 600). The API token itself is only used here and never stored.
set -euo pipefail

API="${CF_API_BASE:-https://api.cloudflare.com/client/v4}"
TUNNEL_NAME="${BORDO_TUNNEL_NAME:-bordo}"
SESSION_DURATION="${CF_SESSION_DURATION:-24h}"
TOKEN_DURATION="${CF_TOKEN_DURATION:-8760h}"
ENV_FILE="${BORDO_ENV_FILE:-$(cd "$(dirname "$0")/.." && pwd)/bordo/.env}"
ROTATE=0
DRY=0
for a in "$@"; do
  case "$a" in
    --rotate-token) ROTATE=1 ;;
    --dry-run) DRY=1 ;;
    -h|--help) sed -n '2,32p' "$0"; exit 0 ;;
    *) echo "unknown argument: $a" >&2; exit 2 ;;
  esac
done

die()  { echo "error: $*" >&2; exit 1; }
info() { echo "▸ $*" >&2; }

command -v jq   >/dev/null || die "jq is required (apt install jq / brew install jq)"
command -v curl >/dev/null || die "curl is required"
: "${CF_API_TOKEN:?set CF_API_TOKEN (see the header of this script for the permissions it needs)}"
: "${BORDO_CONSOLE_HOST:?set BORDO_CONSOLE_HOST, e.g. bordo.example.com}"
: "${BORDO_API_HOST:?set BORDO_API_HOST, e.g. bordo-api.example.com}"
: "${BORDO_ACCESS_EMAILS:?set BORDO_ACCESS_EMAILS, comma separated emails allowed to open the console}"
[ "$BORDO_CONSOLE_HOST" != "$BORDO_API_HOST" ] || die "BORDO_CONSOLE_HOST and BORDO_API_HOST must differ"

# cf METHOD PATH [JSON_BODY]: call the API, fail with Cloudflare's own error text, print .result.
cf() {
  local method="$1" path="$2" body="${3:-}" out
  if [ "$DRY" = 1 ] && [ "$method" != "GET" ]; then
    info "dry-run: $method $path"; echo '{}'; return 0
  fi
  if [ -n "$body" ]; then
    out=$(curl -sS -X "$method" "$API$path" -H "Authorization: Bearer $CF_API_TOKEN" -H "Content-Type: application/json" --data "$body")
  else
    out=$(curl -sS -X "$method" "$API$path" -H "Authorization: Bearer $CF_API_TOKEN")
  fi
  if [ "$(printf '%s' "$out" | jq -r '.success // false' 2>/dev/null)" != "true" ]; then
    die "$method $path failed: $(printf '%s' "$out" | jq -c '.errors // .' 2>/dev/null || printf '%s' "$out")"
  fi
  printf '%s' "$out" | jq -c '.result'
}

info "Checking the API token"
cf GET /user/tokens/verify >/dev/null

# ── account and zone ────────────────────────────────────────────────────────────────────────────
ACCOUNT="${CF_ACCOUNT_ID:-}"
if [ -z "$ACCOUNT" ]; then
  accounts=$(cf GET /accounts)
  [ "$(echo "$accounts" | jq length)" = 1 ] || die "token can see several accounts; set CF_ACCOUNT_ID. Found: $(echo "$accounts" | jq -c '[.[]|{id,name}]')"
  ACCOUNT=$(echo "$accounts" | jq -r '.[0].id')
fi

ZONE="${CF_ZONE_ID:-}"
if [ -z "$ZONE" ]; then
  # Try the longest parent domain of the console host that is a zone on this account.
  name="$BORDO_CONSOLE_HOST"
  while [[ "$name" == *.* ]]; do
    name="${name#*.}"
    z=$(cf GET "/zones?name=$name&account.id=$ACCOUNT" | jq -r '.[0].id // empty')
    if [ -n "$z" ]; then ZONE="$z"; ZONE_NAME="$name"; break; fi
  done
  [ -n "$ZONE" ] || die "no Cloudflare zone found for $BORDO_CONSOLE_HOST; add the domain to Cloudflare or set CF_ZONE_ID"
  info "Using zone $ZONE_NAME"
fi

# ── tunnel ──────────────────────────────────────────────────────────────────────────────────────
TUNNEL=$(cf GET "/accounts/$ACCOUNT/cfd_tunnel?name=$TUNNEL_NAME&is_deleted=false" | jq -r '.[0].id // empty')
if [ -z "$TUNNEL" ]; then
  info "Creating tunnel '$TUNNEL_NAME'"
  TUNNEL=$(cf POST "/accounts/$ACCOUNT/cfd_tunnel" "$(jq -nc --arg n "$TUNNEL_NAME" '{name:$n, config_src:"cloudflare"}')" | jq -r '.id // empty')
  [ -n "$TUNNEL" ] || [ "$DRY" = 1 ] || die "tunnel creation returned no id"
else
  info "Reusing tunnel '$TUNNEL_NAME' ($TUNNEL)"
fi
TUNNEL="${TUNNEL:-dry-run-tunnel}"

info "Routing $BORDO_CONSOLE_HOST -> console and $BORDO_API_HOST -> bordod"
cf PUT "/accounts/$ACCOUNT/cfd_tunnel/$TUNNEL/configurations" "$(jq -nc --arg c "$BORDO_CONSOLE_HOST" --arg a "$BORDO_API_HOST" '
  {config:{ingress:[
    {hostname:$c, service:"http://bordo-console:80"},
    {hostname:$a, service:"http://bordod:7401"},
    {service:"http_status:404"}
  ]}}')" >/dev/null
TUNNEL_TOKEN=$(cf GET "/accounts/$ACCOUNT/cfd_tunnel/$TUNNEL/token" | jq -r '.')

# ── DNS ─────────────────────────────────────────────────────────────────────────────────────────
for host in "$BORDO_CONSOLE_HOST" "$BORDO_API_HOST"; do
  rec=$(cf GET "/zones/$ZONE/dns_records?name=$host" | jq -c '.[0] // empty')
  body=$(jq -nc --arg n "$host" --arg t "$TUNNEL.cfargotunnel.com" '{type:"CNAME", name:$n, content:$t, proxied:true, ttl:1, comment:"Bordo tunnel"}')
  if [ -z "$rec" ]; then
    info "DNS: creating $host"
    cf POST "/zones/$ZONE/dns_records" "$body" >/dev/null
  else
    [ "$(echo "$rec" | jq -r .type)" = "CNAME" ] || die "$host already has a $(echo "$rec" | jq -r .type) record; remove it first so it is not overwritten"
    info "DNS: updating $host"
    cf PUT "/zones/$ZONE/dns_records/$(echo "$rec" | jq -r .id)" "$body" >/dev/null
  fi
done

# ── service token (machines) ────────────────────────────────────────────────────────────────────
SVC_NAME="bordo-machines"
existing_id=""
[ -f "$ENV_FILE" ] && existing_id=$(grep -E '^BORDO_CF_ACCESS_CLIENT_ID=' "$ENV_FILE" | cut -d= -f2- || true)
existing_secret=""
[ -f "$ENV_FILE" ] && existing_secret=$(grep -E '^BORDO_CF_ACCESS_CLIENT_SECRET=' "$ENV_FILE" | cut -d= -f2- || true)

svc=$(cf GET "/accounts/$ACCOUNT/access/service_tokens" | jq -c --arg n "$SVC_NAME" '[.[]|select(.name==$n)][0] // empty')
if [ -z "$svc" ]; then
  info "Creating service token '$SVC_NAME' (valid $TOKEN_DURATION)"
  svc=$(cf POST "/accounts/$ACCOUNT/access/service_tokens" "$(jq -nc --arg n "$SVC_NAME" --arg d "$TOKEN_DURATION" '{name:$n, duration:$d}')")
  CLIENT_ID=$(echo "$svc" | jq -r '.client_id // "dry-run"'); CLIENT_SECRET=$(echo "$svc" | jq -r '.client_secret // "dry-run"')
  SVC_ID=$(echo "$svc" | jq -r '.id // "dry-run"')
elif [ "$ROTATE" = 1 ] || [ -z "$existing_secret" ] || [ "$existing_id" != "$(echo "$svc" | jq -r .client_id)" ]; then
  # A secret is only ever shown once, at creation; without it in the env file the only option is a new one.
  info "Rotating service token '$SVC_NAME' (the previous secret stops working)"
  SVC_ID=$(echo "$svc" | jq -r .id)
  rot=$(cf POST "/accounts/$ACCOUNT/access/service_tokens/$SVC_ID/rotate")
  CLIENT_ID=$(echo "$rot" | jq -r '.client_id // "dry-run"'); CLIENT_SECRET=$(echo "$rot" | jq -r '.client_secret // "dry-run"')
else
  info "Reusing service token '$SVC_NAME'"
  SVC_ID=$(echo "$svc" | jq -r .id); CLIENT_ID="$existing_id"; CLIENT_SECRET="$existing_secret"
fi

# ── Access policies (reusable) ──────────────────────────────────────────────────────────────────
upsert_policy() { # name json-body -> id
  local name="$1" body="$2" id
  id=$(cf GET "/accounts/$ACCOUNT/access/policies" | jq -r --arg n "$name" '[.[]|select(.name==$n)][0].id // empty')
  if [ -z "$id" ]; then
    cf POST "/accounts/$ACCOUNT/access/policies" "$body" | jq -r '.id // "dry-run"'
  else
    cf PUT "/accounts/$ACCOUNT/access/policies/$id" "$body" | jq -r '.id // "dry-run"'
  fi
}

emails_json=$(echo "$BORDO_ACCESS_EMAILS" | tr ',' '\n' | sed 's/^ *//; s/ *$//' | grep -v '^$' | jq -R '{email:{email:.}}' | jq -sc .)
[ "$emails_json" != "[]" ] || die "BORDO_ACCESS_EMAILS has no usable addresses"

info "Access policies"
P_PEOPLE=$(upsert_policy "bordo-console-people" "$(jq -nc --argjson inc "$emails_json" '{name:"bordo-console-people", decision:"allow", include:$inc}')")
P_MACHINES=$(upsert_policy "bordo-api-machines" "$(jq -nc --arg id "$SVC_ID" '{name:"bordo-api-machines", decision:"non_identity", include:[{service_token:{token_id:$id}}]}')")

# ── Access applications ─────────────────────────────────────────────────────────────────────────
upsert_app() { # host name policy-id extra-json
  local host="$1" name="$2" pol="$3" extra="$4" id body
  body=$(jq -nc --arg h "$host" --arg n "$name" --arg p "$pol" --arg s "$SESSION_DURATION" --argjson x "$extra" \
    '{type:"self_hosted", name:$n, domain:$h, session_duration:$s, policies:[$p]} + $x')
  id=$(cf GET "/accounts/$ACCOUNT/access/apps?per_page=100" | jq -r --arg h "$host" '[.[]|select(.domain==$h)][0].id // empty')
  if [ -z "$id" ]; then
    info "Access app: creating $host"; cf POST "/accounts/$ACCOUNT/access/apps" "$body" >/dev/null
  else
    info "Access app: updating $host"; cf PUT "/accounts/$ACCOUNT/access/apps/$id" "$body" >/dev/null
  fi
}
upsert_app "$BORDO_CONSOLE_HOST" "Bordo console" "$P_PEOPLE" '{}'
# Machines get a plain 401 instead of a login redirect.
upsert_app "$BORDO_API_HOST" "Bordo API" "$P_MACHINES" '{"service_auth_401_redirect":true}'

# ── write the env file ──────────────────────────────────────────────────────────────────────────
if [ "$DRY" = 1 ]; then info "dry-run: not writing $ENV_FILE"; exit 0; fi
set_env() { # key value
  local key="$1" val="$2" tmp
  tmp=$(mktemp)
  if [ -f "$ENV_FILE" ]; then grep -v "^${key}=" "$ENV_FILE" > "$tmp" || true; fi
  printf '%s=%s\n' "$key" "$val" >> "$tmp"
  (umask 077; cat "$tmp" > "$ENV_FILE"); rm -f "$tmp"; chmod 600 "$ENV_FILE"
}
mkdir -p "$(dirname "$ENV_FILE")"
set_env CLOUDFLARE_TUNNEL_TOKEN "$TUNNEL_TOKEN"
set_env BORDO_CF_ACCESS_CLIENT_ID "$CLIENT_ID"
set_env BORDO_CF_ACCESS_CLIENT_SECRET "$CLIENT_SECRET"

cat >&2 <<EOF

Done. Written to $ENV_FILE (mode 600).

  1. Start the tunnel (this is the only thing that needs to reach out; no inbound ports):
       docker compose --profile cloudflare up -d
  2. Console:  https://$BORDO_CONSOLE_HOST   (Cloudflare emails a one-time code to the allowed addresses)
  3. CLI, from your machine:
       export BORDO_CF_ACCESS_CLIENT_ID=$CLIENT_ID
       export BORDO_CF_ACCESS_CLIENT_SECRET=<from $ENV_FILE>
       bordo config set server https://$BORDO_API_HOST
  4. Nodes: add the same two variables and BORDO_CP_URL=https://$BORDO_API_HOST to the install-node.sh command.

You can now close ports 80/443 and any others except SSH. Rotate machine credentials with --rotate-token.
EOF
