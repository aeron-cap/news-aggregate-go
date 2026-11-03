# Deploying to EC2

Production runs on one EC2 instance. nginx serves the frontend and proxies `/api/` to the Go API on `127.0.0.1:6767`. The API stores everything in PostgreSQL and shells out to `resnik.py` to score interests.

## Server layout

Everything lives under `/opt/pocket-news`:

```text
/opt/pocket-news/
├── releases/<commit-sha>/     one directory per deploy, newest 5 kept
│   ├── api                    Go API binary
│   ├── refresh                feed refresh job binary
│   ├── frontend/dist/         built frontend, served by nginx
│   ├── internal/database/resnik.py
│   ├── requirements.txt
│   └── .env -> ../../shared/.env
├── shared/                    survives across releases
│   ├── .env                   production config (see below)
│   ├── .venv/                 Python venv for resnik.py
│   └── nltk_data/             WordNet corpora
└── current -> releases/<commit-sha>
```

The systemd unit runs `current/api` with `current` as its working directory, so the API finds `.env` and `internal/database/resnik.py` through relative paths.

Two Linux users are involved:

- `pocketnews` runs the API. It needs read access only.
- `deploy` owns `/opt/pocket-news` and is the user the deploy workflow logs in as over SSH.

## Environment variables

The API reads `.env` from its working directory. Variables already set in the process environment, such as the ones in the systemd unit, win over the file.

| Variable | Required | Description |
| --- | --- | --- |
| `DATABASE_URL` | yes | PostgreSQL URL, e.g. `postgresql://news_aggregator@127.0.0.1:5432/news_aggregator?sslmode=disable`. Also passed to `resnik.py`. |
| `PGPASSWORD` | if the role has a password | Password for the role in `DATABASE_URL`. Single-quote it in `.env` if it has special characters. |
| `PYTHON_BIN` | yes in production | Python used to run `resnik.py`. Set to `/opt/pocket-news/shared/.venv/bin/python` in the systemd unit. |
| `NLTK_DATA` | yes in production | Where the WordNet corpora are installed. Set to `/opt/pocket-news/shared/nltk_data` in the systemd unit. |
| `ADDR` | no | API listen address as `host:port`. Defaults to `127.0.0.1:6767`. Keep it on loopback so traffic always goes through nginx. |

`/opt/pocket-news/shared/.env` usually only needs `DATABASE_URL`, `PGPASSWORD`, and `ADDR`. See [`.env.example`](../.env.example).

## One-time server setup

Ubuntu commands, run as a sudo user.

1. Install packages. Any PostgreSQL 13 or newer works (CI uses 16) and so does Python 3.12 or newer.

   ```sh
   sudo apt install nginx postgresql python3-venv
   ```

2. Create the database and role. The API creates the tables and seed data itself on startup.

   ```sh
   sudo -u postgres createuser --pwprompt news_aggregator
   sudo -u postgres createdb --owner news_aggregator news_aggregator
   ```

3. Create the users and directories.

   ```sh
   sudo useradd --system --no-create-home --shell /usr/sbin/nologin pocketnews
   sudo useradd --create-home --shell /bin/bash --groups pocketnews deploy
   sudo mkdir -p /opt/pocket-news/releases /opt/pocket-news/shared
   sudo chown -R deploy:pocketnews /opt/pocket-news
   ```

4. Write the shared config, readable by the `pocketnews` group only.

   ```sh
   sudo -u deploy nano /opt/pocket-news/shared/.env
   sudo chmod 640 /opt/pocket-news/shared/.env
   ```

5. Create the Python venv and install the NLTK corpora. Deploys keep the venv's packages in sync with `requirements.txt`, but the corpora only need to be downloaded once.

   ```sh
   sudo -u deploy python3 -m venv /opt/pocket-news/shared/.venv
   sudo -u deploy /opt/pocket-news/shared/.venv/bin/pip install -r requirements.txt
   sudo -u deploy /opt/pocket-news/shared/.venv/bin/python -m nltk.downloader \
     -d /opt/pocket-news/shared/nltk_data wordnet wordnet_ic
   ```

6. Install the systemd unit from [`deploy/service/`](../deploy/service/).

   ```sh
   sudo cp deploy/service/pocket-news-api.service /etc/systemd/system/
   sudo systemctl daemon-reload
   sudo systemctl enable pocket-news-api
   ```

   The service starts on the first deploy, once `current` exists. Check it with `systemctl status pocket-news-api` and `journalctl -u pocket-news-api`.

7. Install the nginx config from [`deploy/nginx/pocket-news.conf`](../deploy/nginx/pocket-news.conf).

   ```sh
   sudo cp deploy/nginx/pocket-news.conf /etc/nginx/sites-available/pocket-news.conf
   sudo ln -s /etc/nginx/sites-available/pocket-news.conf /etc/nginx/sites-enabled/
   sudo rm -f /etc/nginx/sites-enabled/default
   sudo nginx -t && sudo systemctl reload nginx
   ```

   The session cookie is `Secure`, so browsers only keep it over HTTPS. Point a domain at the instance and add a certificate (certbot) before using the app.

8. Let the deploy user restart the service and reload nginx without a password, and nothing else:

   ```sh
   echo 'deploy ALL=(root) NOPASSWD: /usr/bin/systemctl restart pocket-news-api, /usr/bin/systemctl reload nginx' \
     | sudo tee /etc/sudoers.d/pocket-news-deploy
   sudo chmod 440 /etc/sudoers.d/pocket-news-deploy
   sudo visudo -c
   ```

## Migrating an existing database

Skip this for a new, empty database.

Databases created by older versions use integer IDs. Current versions use UUIDs. On an old database the API refuses to start and logs:

```text
database column users.id must be UUID; run migrations/001_uuid_ids.sql before starting
```

The migration has to be run by hand, once, before the first deploy of the UUID version. It's not part of the release tarball, so run it from a checkout of the repo:

```sh
sudo systemctl stop pocket-news-api
pg_dump "$DATABASE_URL" > pocket-news-before-uuid.sql
psql "$DATABASE_URL" -v ON_ERROR_STOP=1 -f migrations/001_uuid_ids.sql
sudo systemctl start pocket-news-api
```

The script runs in one transaction and refuses to run against a database that already uses UUIDs.
