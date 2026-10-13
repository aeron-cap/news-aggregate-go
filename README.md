# <NAME_HERE> (i dont have a name for it yet) is a link aggregator for tech news.

> This is a very early stage project, I still need to ask permission for some of the sources I want to fetch, and I need to add more features to make it more useful and reliable.

Currently its a simple Go server that scrapes tech news from various sources and it serves through a REST API. The goal is to create a simple and efficient way to get the latest tech news from multiple sources in one place.

It uses SQLite as the database to store the sources, articles fetched, and interests that influence which articles are kept, Python for a NLP scoring pipeline using Wordnet semantic distance, and Go for the server and fetching.

Feel free to contribute by opening issues or pull requests, but please be aware that this is a very early stage project and there are many things that need to be done before it can be considered stable or reliable.

Rough roadmap:
- [x] Frontend
- [x] Tests
- [ ] Add more sources
- [ ] Improve NLP scoring
- [x] Attach preferences to cookies

## Local development

Run the Go API with `go run ./cmd/api`, then run `npm run dev` inside `frontend`.
The frontend uses `/api` requests through Vite's proxy to port 6767. It establishes
an anonymous session before loading personal data. Direct API callers must first
POST to `/set-token` and retain the returned cookie. `/update-token` renews a
still-valid session while preserving the reader's profile.

`DB_PATH` overrides the default `internal/database/news.db`. Startup checks the
database's columns and rejects older schemas with a migration message; it does
not migrate or delete existing data. Use a fresh path for the new schema if you
have not migrated your previous SQLite database.

Topic and source definitions are shared. Selections, enabled subscriptions,
article scores, and read history are personal. Editing a source URL switches
only the current reader's subscription; changing a shared source's name without
changing its URL is rejected. Articles are collected before ranking, and the
newspaper returns up to ten unread articles with duplicate URLs removed.

Python scoring requires `nltk` and its WordNet resources. `PYTHON_BIN` selects the
Python executable used by Go. Go passes the current reader and actual database
path to the scoring script; a manual invocation has this form:

```text
python internal/database/resnik.py --user-id <reader-id> --db <database-path>
```

Run checks with `go test ./...`, `python -m unittest discover -s internal/database
-p '*_test.py'`, and the frontend's `npm test`, `npm run build`, and `npm run lint`.
