# Bordo

**Your own software factory — in a box.** Describe what you want in plain English, and Bordo builds it, ships it, and runs it on your own server.

Big companies have whole platform teams so their engineers can go from idea to a running service without touching servers. Bordo gives a solo builder or a small team that same superpower — self-hosted, on any machine you own, driven by an **AI agent** instead of a wall of dashboards.

You don't need to know Docker, Kubernetes, or CI/CD. You talk to Bordo (or run a couple of simple `bordo` commands) and it handles the rest.

---

## The idea in one picture

```
You:   "create a java web service called hello-api and ship it"
Bordo:  ✓ scaffolds the project
        ✓ builds a container image   (in GitHub Actions)
        ✓ deploys it to your server  (on Kubernetes, set up for you)
        ✓ gives you a public URL
```

You never see the plumbing unless you want to.

---

## Get started (about 5 minutes)

You need one thing: a Linux server you can SSH into (a $5/mo VPS is plenty) with **Docker** installed. Everything else, Bordo sets up.

**1. Install the Bordo CLI** (on the server):
```bash
curl -fsSL https://raw.githubusercontent.com/hasangenc0/bordo/master/install-cli.sh | sh
```

**2. Start Bordo:**
```bash
bordo platform install      # downloads and configures everything
bordo platform up           # starts it
```

**3. Set it up** (one-time; you'll paste a code it prints and a DeepSeek API key for the AI):
```bash
bordo platform token        # prints a setup code
bordo setup                 # follow the prompts
```

**4. Connect GitHub** (so Bordo can build your code). It prints a short code — enter it at the link, from any browser:
```bash
bordo github login
```

That's it. Bordo is running.

---

## Build and ship your first app

**The easy way — just ask the agent:**
```bash
bordo agent chat
› create a java web service called hello-api and build it
› deploy hello-api and expose it to the internet
```
The agent scaffolds the project, builds the image, deploys it, and tells you the URL. You can keep chatting — "show me the logs", "roll it back", "what's running?".

**The one-command way — if you prefer typing commands:**
```bash
bordo project create hello-api --template java-web-service
bordo build trigger hello-api
bordo deploy create <project-id> <image> --region <your-region> --expose nodeport
```

Either way, your app ends up live on a URL like `http://your-server:3xxxx/...`.

---

## Put it on the internet

Bordo runs your apps on lightweight Kubernetes (k3s) that it installs for you. To turn a server into a place Bordo can deploy to:

```bash
bordo region add my-server        # prints a one-line command to run on the target machine
```
Run that one line on the machine, and it joins your fleet — no SSH keys handed around, no manual Kubernetes setup. From then on you deploy there just by naming it.

Want a real domain with HTTPS? Deploy with `--expose ingress --host app.yourdomain.com` and point your domain at the server.

---

## What you can build

- **Java** web services, workers, Kafka consumers/producers, scheduled jobs, and a database layer for any SQL database
- **TypeScript / React** front-ends and Node back-ends

Each comes from a ready-made template, so a new service is one sentence away.

---

## Day-to-day commands

| You want to… | Do this |
|---|---|
| Talk to Bordo | `bordo agent chat` |
| See your projects | `bordo project list` |
| Ship a new version | `bordo build trigger <app>` then `bordo deploy create …` |
| Check what's running | ask the agent, or `bordo deploy status` |
| Start / stop / update Bordo itself | `bordo platform up` / `down` / `upgrade` |

---

## Under the hood (for the curious)

You don't need any of this to use Bordo, but if you're wondering:

- **One self-hosted control plane** (`bordod`) plus an AI agent and a web console — run as containers, managed entirely by the `bordo` CLI.
- **Builds** run in **GitHub Actions** and publish container images to GitHub's registry (GHCR) — so your server never has to build anything heavy.
- **Deploys** run on **k3s** (lightweight Kubernetes) that Bordo installs on your machines. It uses a **hardened, pull-based** model: servers join with a one-time token, the control plane never stores SSH keys or cluster-admin credentials, and each cluster pulls and applies its own config with least-privilege access. See [`docs/architecture/ADR-0006-hardened-deployment.md`](docs/architecture/ADR-0006-hardened-deployment.md).
- **Multi-region:** add as many servers/regions as you want; the control plane manages the whole fleet.

| Level | Technologies |
|---|---|
| Platform | Go · k3s / Kubernetes · Docker · GitHub Actions |
| App runtimes | Java · TypeScript / Node / React |
| State | SQLite (single node) → Postgres (HA) |
| Observability | OpenTelemetry · VictoriaMetrics · Loki · Tempo |

Full CLI walkthrough: [`docs/guides/running-with-the-cli.md`](docs/guides/running-with-the-cli.md).
Building from source / contributing: [`docs/guides/getting-started.md`](docs/guides/getting-started.md) and [`CONTRIBUTING.md`](CONTRIBUTING.md).

---

## Project status

**Working today, end to end:** install → connect GitHub → create a project → build → deploy to Kubernetes → served on a public URL, all driven by the CLI and agent. The hardened deployment model (ADR-0006) is implemented and validated live. Active development; contributions welcome.

## License

Apache 2.0 — see [`LICENSE`](LICENSE).
