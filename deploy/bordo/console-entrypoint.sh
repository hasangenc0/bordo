#!/bin/sh
set -eu

# Fail closed: nginx injects the agent token into proxied requests, so serving
# the console without a login would hand the agent to anyone who can reach it.
if [ -z "${BORDO_CONSOLE_PASSWORD:-}" ] || [ "${#BORDO_CONSOLE_PASSWORD}" -lt 12 ]; then
  echo "BORDO_CONSOLE_PASSWORD must be set (12+ characters); refusing to serve the console without a login." >&2
  exit 1
fi
# Password goes in over stdin so it never appears in a process list.
hash=$(printf '%s' "$BORDO_CONSOLE_PASSWORD" | openssl passwd -apr1 -stdin)
printf 'bordo:%s\n' "$hash" > /etc/nginx/.htpasswd
chmod 644 /etc/nginx/.htpasswd

# Substitute only ${BORDO_AGENT_TOKEN}; leave nginx's own $variables untouched.
envsubst '${BORDO_AGENT_TOKEN}' \
  < /etc/nginx/templates/default.conf.template \
  > /etc/nginx/http.d/default.conf
exec nginx -g 'daemon off;'
