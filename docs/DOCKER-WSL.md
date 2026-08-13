# Docker in the terminal, without Docker Desktop

Docker Desktop is not required. The engine runs inside WSL2, and `docker` /
`docker compose` behave exactly as they do anywhere else.

Your machine already has what this needs: **WSL2**, **Ubuntu installed**, and
virtualization enabled.

One thing to be clear about up front: **Docker installs system-wide, not into
the project.** Nothing is added to the repository. Once it is installed,
`docker compose up` from the repository root brings up the whole stack — that
part already works and needs no changes.

---

## Where the repository lives

**Use the checkout you already have.** WSL reaches it at:

```
/mnt/c/Users/alimi/Desktop/webdev/football-database-app
```

One copy, one place to edit, and `.env` is already there with your JWT keys.

The cost is build speed. Docker reads thousands of files during a build, and
from `/mnt/c` every read crosses the Windows↔Linux boundary — expect a few
minutes for a first build rather than under one, and slower rebuilds after a
code change.

A second checkout inside WSL (`~/scoutpulse`) builds much faster, but then two
copies exist and neither is obviously the real one: edits, uncommitted changes
and branches all have to be kept in step by hand. That trade only pays off if
you are rebuilding images constantly. Bringing the stack up occasionally to
look at it does not qualify.

---

## Step 2 — Enter WSL

From PowerShell:

```powershell
wsl -d Ubuntu
```

Your prompt changes to something like `alimi@machine:~$`. Everything from here
until Step 6 is typed inside that Linux shell.

---

## Step 3 — Install Docker Engine

Paste this as one block. It adds Docker's official apt repository and installs
the engine plus the Compose plugin.

```bash
# Remove any old distro-packaged versions
for p in docker.io docker-doc docker-compose podman-docker containerd runc; do
  sudo apt-get remove -y $p 2>/dev/null
done

# Docker's official GPG key and repository
sudo apt-get update
sudo apt-get install -y ca-certificates curl
sudo install -m 0755 -d /etc/apt/keyrings
sudo curl -fsSL https://download.docker.com/linux/ubuntu/gpg -o /etc/apt/keyrings/docker.asc
sudo chmod a+r /etc/apt/keyrings/docker.asc

echo "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.asc] \
https://download.docker.com/linux/ubuntu $(. /etc/os-release && echo "$VERSION_CODENAME") stable" \
  | sudo tee /etc/apt/sources.list.d/docker.list > /dev/null

sudo apt-get update
sudo apt-get install -y docker-ce docker-ce-cli containerd.io \
  docker-buildx-plugin docker-compose-plugin
```

---

## Step 4 — Run docker without sudo, and start it automatically

```bash
sudo usermod -aG docker $USER
```

Then enable systemd so the daemon starts by itself, instead of you running
`sudo service docker start` every session:

```bash
sudo tee /etc/wsl.conf > /dev/null <<'EOF'
[boot]
systemd=true
EOF
```

Now restart WSL. Run this from **PowerShell**, not inside WSL:

```powershell
wsl --shutdown
```

Then go back in:

```powershell
wsl -d Ubuntu
```

Verify:

```bash
docker run --rm hello-world
```

If that prints "Hello from Docker!", you are done installing.

---

## Step 5 — Go to the project

```bash
cd /mnt/c/Users/alimi/Desktop/webdev/football-database-app
```

Confirm your keys are present — compose refuses to start without them:

```bash
grep -c JWT_PRIVATE_KEY .env
```

Must print `1`. If it prints `0`, generate them:

```bash
cd libs/auth && go run ./cmd/genkeys >> ../../.env && cd ../..
```

(That needs Go inside WSL: `sudo apt-get install -y golang-go`. It is only
required if `.env` is missing its keys — on this machine it already has them,
written by `scripts/dev-setup.ps1`.)

---

## Step 6 — Run the whole stack

From the repository root:

```bash
docker compose up
```

That is the "run it in root and it works" you were after. It starts:

| Service | Where |
|---|---|
| gateway (Caddy) | http://localhost:8000 |
| identity-svc | behind the gateway at `/api/identity` |
| football-svc | behind the gateway at `/api/football` |
| postgres | internal |
| nats | internal |

Everything enters through **port 8000**:

```
http://localhost:8000/api/identity/api/v1/auth/login
http://localhost:8000/api/football/api/v1/players
```

Check it:

```bash
curl http://localhost:8000/health
curl http://localhost:8000/api/football/api/v1/leagues
```

Stop it with `Ctrl+C`, or from another terminal:

```bash
docker compose down
```

---

## What this gives you that the local route does not

`scripts/dev-run.ps1` runs the two services directly and is fine for API work,
but it cannot exercise:

- **The gateway.** Path routing, the correlation id assigned at the edge, and
  the blocks that stop `/metrics` being served publicly.
- **NATS.** Without it, events are disabled. With it, `player.transferred` and
  the rest actually publish.
- **Startup ordering.** Migrations complete, then services become healthy, then
  the gateway starts.

---

## Adding the frontend later

The frontend is already in `docker-compose.yml`, behind a profile so it does
not build while it is still a placeholder. When you start building it:

```bash
docker compose --profile frontend up
```

It comes up on **http://localhost:4200** and talks to the gateway on 8000 —
which is the whole reason the gateway exists. The frontend needs to know one
origin, not a port per service.

---

## If something goes wrong

| What you see | Fix |
|---|---|
| `permission denied while trying to connect to the Docker daemon` | The group change needs a restart: `wsl --shutdown` from PowerShell, then re-enter |
| `Cannot connect to the Docker daemon` | systemd is not running. Check `/etc/wsl.conf` from Step 4, then `wsl --shutdown` |
| `JWT_PRIVATE_KEY: run "make keys"...` | `.env` has no keys. Redo the genkeys line in Step 5 |
| Builds are extremely slow | The repo is on `/mnt/c`. See Step 1, option A |
| `port is already allocated` | Something is using 5432, 8000, 8080 or 8081 — often the local Postgres or a `dev-run.ps1` service still running on Windows |
| `docker: command not found` after install | You are in PowerShell, not WSL. Run `wsl -d Ubuntu` first |

The last one is worth watching: your Windows PostgreSQL listens on 5432, and
compose publishes 5432 too. If they clash, either stop the Windows service
while using Docker, or change the host side of the mapping in
`docker-compose.yml` to `127.0.0.1:5433:5432`.
