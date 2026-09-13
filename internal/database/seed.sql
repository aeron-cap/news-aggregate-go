INSERT INTO sources (name, url, is_active) VALUES
    ('Lobsters', 'https://lobste.rs/rss', 1),
    ('Hacker News', 'https://hnrss.org/frontpage', 1),
    ('Hacker News Atom', 'https://hnrss.org/frontpage.atom', 1),
    ('Hacker News Points', 'https://hnrss.org/show?points=25', 1),
    ('Hacker News RSS', 'https://news.ycombinator.com/rss', 1),
    ('Dev.to', 'https://dev.to/feed', 1),
    ('Simon Willison Everything', 'https://simonwillison.net/atom/everything/', 1),
    ('Simon Willison Links', 'https://simonwillison.net/atom/links/', 1),
    ('Daring Fireball', 'https://daringfireball.net/feeds/main', 1),
    ('Waxy.org', 'https://waxy.org/feed/', 1),
    ('Kottke.org', 'https://feeds.kottke.org/main', 1),
    ('Dan Luu', 'https://danluu.com/atom.xml', 1),
    ('Eli Bendersky', 'https://eli.thegreenplace.net/feeds/all.atom.xml', 1),
    ('Brandur Leach', 'https://brandur.org/articles.atom', 1)
ON CONFLICT DO NOTHING;