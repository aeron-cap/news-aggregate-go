# <NAME_HERE> (i dont have a name for it yet) is a link aggregator for tech news.

> This is a very early stage project, I still need to ask permission for some of the sources I want to fetch, and I need to add more features to make it more useful and reliable.

Currently its a simple Go server that scrapes tech news from various sources and it serves through a REST API. The goal is to create a simple and efficient way to get the latest tech news from multiple sources in one place.

It uses PostgreSQL to store sources, articles, sessions, and reader preferences, Python for a WordNet scoring pipeline, and Go for the server and fetching.

Feel free to contribute by opening issues or pull requests, but please be aware that this is a very early stage project and there are many things that need to be done before it can be considered stable or reliable.

Feed fetching accepts public HTTP/HTTPS destinations on their default ports.
The client validates saved URLs and redirects, checks all DNS answers, and
connects directly to validated IPs. Private, loopback, link-local, and selected
special-purpose ranges are blocked, including IPv4-mapped IPv6 addresses.
Environment proxies and HTTPS downgrade redirects are disabled. Each request
has an eight-second timeout and a 5 MiB limit on the decompressed response.
The conservative address policy in `internal/feed/http_client.go` should be
maintained against the [IANA IPv4](https://www.iana.org/assignments/iana-ipv4-special-registry/)
and [IPv6](https://www.iana.org/assignments/iana-ipv6-special-registry/) registries.

Rough roadmap:
- [x] Frontend
- [x] Tests
- [ ] Add more sources
- [ ] Improve NLP scoring
- [x] Attach preferences to cookies
