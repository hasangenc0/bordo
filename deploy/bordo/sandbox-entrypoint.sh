#!/bin/sh
# Egress policy for the code sandbox, applied before the server starts.
#
# The sandbox runs repo code the agent did not write, and it needs the internet to download
# dependencies. It must not use that access to reach the Docker host, other containers, the LAN or
# the cloud metadata service (169.254.169.254), all of which sit behind private or link-local
# addresses. So: allow replies to connections that came in (the agent's requests), allow loopback
# (Docker's embedded DNS lives there), drop every private range, allow the rest (public internet).
#
# Fails closed: if the rules cannot be installed the container exits instead of running open.
# BORDO_SANDBOX_ALLOW_CIDRS adds exceptions (space separated), e.g. a private package mirror.
# BORDO_SANDBOX_EGRESS=open skips the policy; BORDO_SANDBOX_DRYRUN=1 prints the commands instead.
set -eu

# The root filesystem is read-only in compose; iptables -w keeps its lock file here instead of /run.
export XTABLES_LOCKFILE=/tmp/xtables.lock

ipt() {
  if [ "${BORDO_SANDBOX_DRYRUN:-}" = "1" ]; then
    echo "$@"
  else
    "$@"
  fi
}

apply_v4() {
  ipt iptables -w -F OUTPUT
  ipt iptables -w -A OUTPUT -o lo -j ACCEPT
  ipt iptables -w -A OUTPUT -m conntrack --ctstate ESTABLISHED,RELATED -j ACCEPT
  for cidr in ${BORDO_SANDBOX_ALLOW_CIDRS:-}; do
    ipt iptables -w -A OUTPUT -d "$cidr" -j ACCEPT
  done
  for cidr in 0.0.0.0/8 10.0.0.0/8 100.64.0.0/10 127.0.0.0/8 169.254.0.0/16 172.16.0.0/12 192.0.0.0/24 192.168.0.0/16 224.0.0.0/4 240.0.0.0/4; do
    ipt iptables -w -A OUTPUT -d "$cidr" -j REJECT
  done
}

apply_v6() {
  command -v ip6tables >/dev/null 2>&1 || return 0
  ipt ip6tables -w -F OUTPUT
  ipt ip6tables -w -A OUTPUT -o lo -j ACCEPT
  ipt ip6tables -w -A OUTPUT -m conntrack --ctstate ESTABLISHED,RELATED -j ACCEPT
  for cidr in ::/128 ::1/128 fc00::/7 fe80::/10 ff00::/8 64:ff9b::/96; do
    ipt ip6tables -w -A OUTPUT -d "$cidr" -j REJECT
  done
}

if [ "${BORDO_SANDBOX_EGRESS:-}" != "open" ]; then
  apply_v4
  apply_v6 || echo "bordo-sandbox: warning: could not install IPv6 egress rules (IPv6 is usually off in Docker)" >&2
  echo "bordo-sandbox: egress policy installed (private, link-local and host addresses blocked)" >&2
else
  echo "bordo-sandbox: WARNING: BORDO_SANDBOX_EGRESS=open, the sandbox can reach private networks" >&2
fi

exec "$@"
