# Installing meth

meth runs as three containers: the engine, PostgreSQL 18, and (in production)
Caddy for HTTPS. Everything is driven by `make`, which wraps `docker compose`.

- `.env` holds the deployment: secrets, the first moderator, the domain.
  It is made from `.env.example` and is never committed.
- `config.yaml` holds the site: title, banner, captcha, post limit, tags,
  webring, About text. It is made from `config.example.yaml`.

## Requirements

- Docker with the compose plugin
- make
- For running the engine outside Docker: Go 1.27.1 or later
  (templ is a Go tool dependency, so `go tool templ` needs no separate install)

---

## Local development

### 1. Configure

```sh
cp .env.example .env
cp config.example.yaml config.yaml
```

Edit `.env` for a local machine:

```sh
POSTGRES_PASSWORD=meth
METH_SECRET=dev
METH_MOD_USERNAME=admin
METH_MOD_PASSWORD=admin
COMPOSE_PROFILES=
METH_ENV=development
METH_TRUSTED_PROXY=1
METH_LOG_LEVEL=debug
```

- `COMPOSE_PROFILES` empty leaves Caddy off, so nothing tries to take ports
  80 and 443 or ask for a certificate.
- `METH_ENV=development` keeps cookies usable over plain `http://`.
- To post without solving captchas while developing, set `captcha: false`
  in `config.yaml`. Set `wait: 0` under `captcha_post_limit` to lift the
  post limit too.

### 2. Run in Docker

```sh
make up        # build the image, start the engine and Postgres
make logs      # follow the engine's log
```

The site is at <http://localhost:3000>, the moderator panel at
<http://localhost:3000/mod> (log in with `METH_MOD_USERNAME` /
`METH_MOD_PASSWORD`; the account is created on first start).

Day to day:

| command | what it does |
|---|---|
| `make up` | rebuild and restart after changing Go code, templates or CSS |
| `make dev` | the same, in the foreground with logs (Ctrl-C stops it) |
| `make restart` | restart the engine after editing `config.yaml` (no rebuild) |
| `make check` | validate `config.yaml` without restarting |
| `make shell` | psql into the database |
| `make down` | stop everything; posts are kept |
| `make reset` | stop everything and delete the database (asks first) |

### 3. Or run the engine natively

Faster for working on the code: Postgres in Docker, the engine on the host.

```sh
make db        # start only Postgres, on 127.0.0.1:5434
make run       # generate templates, build bin/meth, run it with .env
```

`make run` points `DATABASE_URL` at the compose database using
`POSTGRES_PASSWORD` from `.env`. Stop it with Ctrl-C.

Other targets for working on the code:

```sh
make generate  # regenerate *_templ.go after editing view/*.templ
make test      # generate, then run every test
make vet       # generate, gofmt check, go vet
make fmt       # gofmt and templ fmt in place
```

Migrations in `db/migrations` run automatically when the engine starts.

---

## Production deployment

### 1. Prepare the server

- A Linux server with Docker, the compose plugin, make and git.
- A domain whose DNS A (and AAAA) record points at the server.
- Ports 80 and 443 (TCP, and 443/UDP for HTTP/3) open to the internet.
  Caddy needs both to obtain and renew the certificate.

The engine and the database listen on `127.0.0.1` only; visitors reach the
site through Caddy.

### 2. Get the code

```sh
git clone <this repository> meth
cd meth
```

### 3. First `make deploy`: create `.env`

```sh
make deploy
```

The first run copies `.env.example` to `.env` and stops. Fill it in:

```sh
POSTGRES_PASSWORD=<openssl rand -hex 32>
METH_SECRET=<openssl rand -hex 32>
METH_MOD_USERNAME=<first moderator>
METH_MOD_PASSWORD=<a strong password>
COMPOSE_PROFILES=proxy
METH_DOMAIN=your.domain
METH_ENV=production
METH_TRUSTED_PROXY=1
```

- Generate each secret with `openssl rand -hex 32`.
- `POSTGRES_PASSWORD` is only read when the database volume is first
  created; changing it later does not change the database's password.
- Changing `METH_SECRET` logs every moderator out.
- `METH_POSTER_ID_SECRET` is optional; when empty, poster IDs are keyed with
  `METH_SECRET`. Changing it changes every future ID.
- Keep `METH_TRUSTED_PROXY=1`: behind Caddy the engine needs it to see
  visitors' real addresses and that requests are HTTPS. Without it every
  post and login is refused.

### 4. Second `make deploy`: create `config.yaml`

```sh
make deploy
```

This run copies `config.example.yaml` to `config.yaml` and stops. Edit the
site's title, description, banner, About text and limits, and add the
public address, which share cards need:

```yaml
url: https://your.domain
```

Keep `captcha: true` in production.

### 5. Third `make deploy`: go live

```sh
make deploy
```

This validates the compose file, builds the image, checks `config.yaml`
inside the new image, starts the engine, Postgres and Caddy, and waits until
the engine reports healthy. A mistake in the configuration stops the deploy
before anything running is touched. If the engine does not come up healthy,
the last lines of its log are printed.

The site is now at `https://your.domain`. Caddy fetches the certificate on
first request and renews it by itself.

Log in at `https://your.domain/mod`, then remove `METH_MOD_USERNAME` and
`METH_MOD_PASSWORD` from `.env`; more moderators can be added from the panel.

### Running it

| command | what it does |
|---|---|
| `make deploy` | rebuild and redeploy; safe to run any time, posts are kept |
| `make update` | `git pull --ff-only`, then `make deploy` |
| `make check` | validate `config.yaml` before a restart |
| `make restart` | apply an edited `config.yaml` (no rebuild) |
| `make health` | ask the engine whether it is healthy, and list the containers |
| `make logs` | follow the engine's log |
| `docker compose logs -f caddy` | follow Caddy's log |

`GET /healthz` answers `200 ok` with the database's ping and round-trip
times while the engine can reach its database, and `503 unavailable`
otherwise. Docker and Caddy both use it.

The containers restart after a crash or a reboot, and stay down after
`make down`. On a stop or a redeploy the engine finishes the requests in
flight before exiting.

### Backups

The posts, bans, filters and moderators live in the `db` volume.

```sh
make backup
```

writes `backups/meth-YYYYMMDD-HHMMSS.dump` (a compressed `pg_dump`, taken
while the site keeps running) and copies `config.yaml` beside it as
`backups/meth-YYYYMMDD-HHMMSS.config.yaml`. `.env` is not copied: it holds
the secrets, so keep it somewhere safe yourself. Restoring onto a new server
with a different `METH_SECRET` works, but logs moderators out, and with a
different `METH_POSTER_ID_SECRET` new posts get different poster IDs.

```sh
make restore                                   # the newest backup
make restore FILE=backups/meth-20261006-153000.dump
```

asks for `yes`, stops the engine, replaces the whole database with the
backup in one transaction, and starts the engine again. If the restore
fails, the database is left as it was. To restore a site's text as well,
copy the matching `.config.yaml` over `config.yaml` and `make restart`.

Copy the `backups/` folder off the server regularly; a backup on the same
disk does not survive losing the disk. `make reset` deletes the volume, so
run `make backup` first.

### Running behind a Caddy on the host instead

To use a Caddy installed on the server rather than the container, leave
`COMPOSE_PROFILES` empty in `.env`, copy `Caddyfile` to
`/etc/caddy/Caddyfile`, set `METH_DOMAIN` in Caddy's environment
(`systemctl edit caddy`, then `Environment=METH_DOMAIN=your.domain`) and run
`systemctl reload caddy`. It proxies to the engine on `127.0.0.1:3000`.
