# Running Bordo with the CLI

The `bordo` CLI is the single control surface for the whole platform. Docker is
the runtime under the hood, but you never type a `docker compose` command. This
guide takes you from a fresh server to a running, configured Bordo.

---

## Prerequisites

- **Docker 24+** with the Compose plugin: `docker compose version` should print `v2.x`.
  The CLI checks for Docker and links the install docs if it's missing; it does
  not install Docker for you (that touches system packages).
- The `bordo` binary (see [Install the CLI](#install-the-cli)).

---

## Install the CLI

One line, no Go toolchain needed (downloads a prebuilt binary):

```bash
curl -fsSL https://raw.githubusercontent.com/hasangenc0/bordo/master/install-cli.sh | sh
bordo version
```

Alternatives: `go install github.com/hasangenc0/bordo/cli/bordo@latest` (needs Go),
or build from source with `make build` (produces `./bin/bordo`).

---

## Shortcut: one command

On the server, `bordo setup` does everything in the next three sections: it
installs and starts the platform if needed, reads the setup token, asks for your
DeepSeek API key, and offers to connect GitHub at the end.

```bash
bordo setup
```

Flags: `--deepseek-key` (or `DEEPSEEK_API_KEY`), `--setup-token` (or
`BORDO_SETUP_TOKEN`), `--skip-github`, and `--advanced` to also set the public
URL, LLM model/endpoint and registry. `curl ... install-cli.sh | BORDO_SETUP=1 sh`
runs it straight after installing. The steps below are the same flow, one at a
time.

---

You can also finish setup in the web console instead of the terminal: after
`bordo platform install` and `bordo platform up`, open `http://<server>:3000` and
follow the setup screen (see `console/README.md`).

---

## Bring up the platform

```bash
bordo platform install    # write config to ~/.bordo/deploy, pull images
bordo platform up         # start bordod + agent + console
```

`install` writes two files to `~/.bordo/deploy`:

- `docker-compose.yml` — embedded in the binary, no git clone needed
- `.env` — generated with a random internal token and default ports

All tunables live in `.env`. Edit it and re-run `bordo platform up` to apply:

```dotenv
BORDO_INTERNAL_TOKEN=<generated>   # shared secret between services — keep private
BORDOD_PORT=7401                   # control-plane API
AGENT_PORT=7402                    # AI agent
CONSOLE_PORT=3000                  # web console
```

---

## Complete first-run setup

On first start bordod prints a one-time **setup token**. Retrieve it, then run the
setup wizard (you'll also need a DeepSeek API key for the agent's LLM):

```bash
bordo platform token    # prints the setup token
bordo setup             # defaults to http://localhost:7401 — no flag needed on the server
```

> From your laptop instead of the server, point at the tunnel/URL:
> `bordo setup --server http://localhost:7401` (with an SSH tunnel) or
> `bordo setup --server http://<SERVER_IP>:7401`.

Setup returns your **admin token**. Save it; it is shown only once.

---

## Connect GitHub

Bordo builds your code in GitHub Actions, so it needs access to your GitHub
account. This uses GitHub's device flow, so nothing opens a browser on the server:

```bash
bordo github login
```

It prints a short code and a link. Open the link in any browser (laptop or phone),
enter the code, and approve. You'll see `✓ GitHub connected`.

> One-time prerequisite: register a public GitHub OAuth App (GitHub → Settings →
> Developer settings → OAuth Apps → New), turn on **Enable Device Flow**, and set
> `export BORDO_GITHUB_CLIENT_ID=<its client id>` before running `bordo github login`.

---

## Create and deploy your first app

**The easy way, ask the agent** (it drives everything via tool calls):

```bash
bordo agent chat
› create a java web service called hello-api and build it
› deploy hello-api to <region> and expose it to the internet
```

The agent scaffolds the project, builds the image in GitHub Actions, deploys it,
and reports the URL. Keep chatting to inspect logs, redeploy, or roll back.

**The explicit way, plain commands:**

```bash
bordo project create hello-api --template go-web-service
bordo build trigger hello-api                       # builds in GitHub Actions → image in GHCR
bordo build status <build-id>                       # wait for "success"

bordo deploy create <project-id> ghcr.io/<owner>/hello-api:<tag> \
  --region <region> --expose nodeport
```

> Templates: `go-web-service`, `go-job`, and `go-kafka` produce static Go binaries
> that run in a few MB of RAM. The `java-*` templates carry a richer SDK. List them
> all with `bordo template list`, or just ask the agent for the kind of app you want.

`--expose` controls public reach:

| Value | Effect |
|---|---|
| `none` (default) | Internal only (inside the cluster) |
| `nodeport` | Reachable at `http://<server-ip>:<nodeport>` |
| `ingress --host app.example.com` | HTTP(S) at a hostname (needs an ingress controller) |

---

## Add a server (region)

A "region" is a machine that runs your apps. Register one and Bordo prints a
single command to run on that machine, with no SSH keys handed around:

```bash
bordo region add my-server            # prints the one-line join command
```

On the target machine, run that printed command. It installs lightweight
Kubernetes (k3s), joins the fleet with a one-time token, and starts pulling its
own config. After that, `--region my-server` on any deploy ships there.

> The join is outbound-only and least-privilege: the control plane never stores
> an SSH key or cluster-admin credentials. See
> [`../architecture/ADR-0006-hardened-deployment.md`](../architecture/ADR-0006-hardened-deployment.md).

---

## Access the console

Keep the services private (admin-only). From your laptop, tunnel over SSH rather
than exposing ports publicly:

```bash
ssh -L 3000:localhost:3000 -L 7401:localhost:7401 root@<SERVER_IP>
```

Then open **http://localhost:3000**, the chat-first web console.

Point the CLI at the same tunnel:

```bash
bordo config set server http://localhost:7401
bordo login          # paste your admin token
bordo whoami
```

---

## Day-to-day operations

| Command | What it does |
|---|---|
| `bordo platform status` | Show service health (`docker compose ps`) |
| `bordo platform logs -f` | Stream logs; add a service name to filter (e.g. `logs -f bordod`) |
| `bordo platform restart [service]` | Restart all services, or one |
| `bordo platform upgrade` | Pull the latest images and restart — **data is preserved** |
| `bordo platform down` | Stop and remove containers; volumes are kept |
| `bordo platform down --volumes` | Stop **and delete all data** — destructive |

---

## Upgrades

```bash
bordo platform upgrade
```

Pulls the newest images from GHCR and restarts services in place. Data volumes
(`bordod-data`, `agent-data`) are never touched.

---

## Where things live

| Path | Purpose |
|---|---|
| `~/.bordo/deploy/docker-compose.yml` | Service definitions (rewritten by `install`) |
| `~/.bordo/deploy/.env` | Ports + internal token (yours to edit) |
| `bordod-data` volume | Control-plane SQLite database |
| `agent-data` volume | Agent chat history |

To change ports or rotate the internal token, edit `.env` (or
`bordo platform install --force` to regenerate it) and run `bordo platform up`.
