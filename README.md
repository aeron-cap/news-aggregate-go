# <NAME_HERE> (i dont have a name for it yet) is a link aggregator for tech news.

> This is a very early stage project, I still need to ask permission for some of the sources I want to fetch, and I need to add more features to make it more useful and reliable.

Currently its a simple Go server that scrapes tech news from various sources and it serves through a REST API. The goal is to create a simple and efficient way to get the latest tech news from multiple sources in one place.

It uses SQLite as the database to store the sources, articles fetched, and interests that influence which articles are kept, Python for a NLP scoring pipeline using Wordnet semantic distance, and Go for the server and fetching.

Feel free to contribute by opening issues or pull requests, but please be aware that this is a very early stage project and there are many things that need to be done before it can be considered stable or reliable.

Rough roadmap:
- [ ] Frontend
- [ ] Tests
- [ ] Add more sources
- [ ] Improve NLP scoring
- [ ] Attach preferences to cookies