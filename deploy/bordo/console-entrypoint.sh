#!/bin/sh
# Substitute only ${BORDO_AGENT_TOKEN} — leave nginx's own $variables untouched.
envsubst '${BORDO_AGENT_TOKEN}' \
  < /etc/nginx/templates/default.conf.template \
  > /etc/nginx/http.d/default.conf
exec nginx -g 'daemon off;'
