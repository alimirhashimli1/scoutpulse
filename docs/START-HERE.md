# Start the app on this machine

Step by step, with the exact commands. Nothing here needs Docker.

Your machine already has everything required: **Go**, **PostgreSQL 18 running on
port 5432**, and **VS Code with the REST Client extension**.

You will end up with **three PowerShell windows open**:

| Window | What it runs | Stays open? |
|---|---|---|
| 1 | Setup, then free for commands | — |
| 2 | identity-svc (accounts, login) | yes, leave it running |
| 3 | football-svc (the actual data) | yes, leave it running |

---

## Step 1 — Open PowerShell in the project folder

Press `Win`, type **PowerShell**, open it, then paste:

```powershell
cd C:\Users\alimi\Desktop\webdev\football-database-app
```

You are in the right place if this prints a list of files including `go.work`:

```powershell
dir
```

---

## Step 2 — Run the setup (only ever once)

```powershell
.\scripts\dev-setup.ps1
```

It will ask:

```
Password for Postgres user 'postgres':
```

Type the password you chose when you installed PostgreSQL and press Enter.
Nothing appears as you type — that is normal.

**What it does:** creates the JWT keys, two database users, two databases
(`identity_db`, `football_db`), and creates all the tables.

**You are done when you see:**

```
Setup complete.
```

You never need to run this again unless you delete the databases.

---

## Step 3 — Start the first service

Open a **new** PowerShell window (`Win` → PowerShell), then:

```powershell
cd C:\Users\alimi\Desktop\webdev\football-database-app
.\scripts\dev-run.ps1 identity
```

**Working when you see:**

```
identity-svc -> http://localhost:8080
{"time":"...","level":"INFO","msg":"connected to database", ...}
{"time":"...","level":"INFO","msg":"server starting","service":"identity-svc","addr":":8080"}
```

**Leave this window open.** The service stops the moment you close it.

---

## Step 4 — Start the second service

Open a **third** PowerShell window:

```powershell
cd C:\Users\alimi\Desktop\webdev\football-database-app
.\scripts\dev-run.ps1 football
```

**Working when you see:**

```
football-svc -> http://localhost:8081
{"time":"...","level":"INFO","msg":"server starting","service":"football-svc","addr":":8081"}
```

You will also see a line saying events are disabled. That is expected — there
is no message broker running locally, and the services are built to run fine
without one.

**Leave this window open too.**

---

## Step 5 — Check both are alive

Back in **window 1**:

```powershell
curl.exe http://localhost:8080/health
curl.exe http://localhost:8081/health
```

Expected:

```
Identity Service is healthy
Football Service is healthy
```

If you get those two lines, **the app is running**.

---

## Step 6 — Create your login and make yourself an admin

Still in window 1. Create an account:

```powershell
Invoke-RestMethod -Method Post -Uri http://localhost:8080/api/v1/auth/register -ContentType application/json -Body '{"username":"scout1","email":"scout1@example.test","password":"local-dev-password"}'
```

> **Use single quotes around the JSON.**
>
> PowerShell rewrites double-quoted strings before the program ever sees them,
> so the `-d "{\"key\":\"value\"}"` style used in most documentation arrives
> mangled and split across several arguments. You get
> `request body is not valid JSON` followed by a nonsense
> `URL rejected: Port number was not a decimal number` — because a fragment of
> your JSON was parsed as the next argument.
>
> Single quotes are passed through literally. The same rule applies if you
> prefer `curl.exe`:
>
> ```powershell
> curl.exe -X POST http://localhost:8080/api/v1/auth/register -H "Content-Type: application/json" -d '{"username":"scout1","email":"scout1@example.test","password":"local-dev-password"}'
> ```
>
> Easiest of all: skip the terminal and click **Send Request** on the Register
> block in `api.http`, where there is no shell quoting to get wrong.

**Use a throwaway password here.** This is a local development database with no
real data in it, and the command ends up in your PowerShell history. Do not use
a password you use anywhere else.

New accounts are always a plain `user` and cannot create anything. Only an
admin can promote someone, so the very first admin has to be made directly in
the database. Use whatever username you registered above:

```powershell
$env:PGPASSWORD='password'
psql -U identity_user -d identity_db -c "UPDATE users SET role='admin' WHERE username='scout1'"
```

Expected output: `UPDATE 1`

---

## Step 7 — Click through the endpoints

Open **`api.http`** in VS Code. Above every request there is a small
**Send Request** link. Click them from the top down.

The order matters the first time, because each section uses ids from the one
before:

1. **Health** — confirms both services answer
2. **Accounts and tokens** — click **Log in**; every request below reuses it
3. Skip the promotion note, you already did it in Step 6
4. **Competitions** → **Seasons** → **Clubs** → **Players** → **Transfers**

Three things worth watching, because they are the point of the whole design:

- In **Players**, the `PUT` that tries to change `team_id` returns 200 but the
  club does **not** change. Send the GET after it to see for yourself.
- In **Transfers**, record the transfer, then send that same GET again. *Now*
  the club has changed — because the history says so.
- In **Error contracts**, `/players/not-a-uuid` returns **400**, not 500.

---

## Stopping everything

Press `Ctrl` + `C` in windows 2 and 3, or just close them.

Nothing is left running in the background. Your data stays in Postgres until
you delete it.

---

## Starting again tomorrow

Setup is already done, so it is only two commands in two windows:

```powershell
cd C:\Users\alimi\Desktop\webdev\football-database-app
.\scripts\dev-run.ps1 identity
```

```powershell
cd C:\Users\alimi\Desktop\webdev\football-database-app
.\scripts\dev-run.ps1 football
```

---

## If something goes wrong

| What you see | What it means | Fix |
|---|---|---|
| `cannot be loaded because running scripts is disabled` | PowerShell is blocking the script | Run `Set-ExecutionPolicy -Scope Process -ExecutionPolicy Bypass` in that window, then try again |
| `password authentication failed for user "postgres"` | Wrong Postgres password in Step 2 | Re-run Step 2 with the right one |
| `psql: not recognized` | psql is not on PATH | Use the full path: `& "C:\Program Files\PostgreSQL\18\bin\psql.exe"` |
| `connection refused` on port 5432 | PostgreSQL is not running | Open **Services**, find `postgresql-x64-18`, click Start |
| `.env not found` when starting a service | Step 2 has not run yet | Run Step 2 |
| `address already in use` | The service is already running in another window | Close the other window, or `Ctrl+C` in it |
| Service starts then exits immediately | Usually the database or keys | Read the last log line — it names the cause |
| `401` on a request in `api.http` | Your token expired (15 minutes) | Send the **Log in** request again |
| `403` on a create | You are not an admin | Do Step 6, then send **Log in** again |
| `409` on register | That username already exists | Change `@user` at the top of `api.http` |

---

## Automatic check instead of clicking

If you want everything exercised at once rather than by hand, with pass/fail
for each:

```powershell
.\scripts\check-endpoints.ps1
```

It does Steps 6 and 7 by itself, asserts on around 35 behaviours, and cleans up
after itself.

---

## The Docker route (not needed here)

If you ever install Docker Desktop, the whole stack — including the gateway on
port 8000 and the message broker — comes up with:

```powershell
docker compose up
```

That path also gives you the API gateway, which the local route above does not.
Everything then lives behind `http://localhost:8000/api/football/...` and
`http://localhost:8000/api/identity/...`.
