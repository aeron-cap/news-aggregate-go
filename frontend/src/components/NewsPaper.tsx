import { useEffect, useState } from 'react'
import { fetchToday } from '../../api'
import '../styles/NewsPaper.css'

interface Article {
  id: number
  title: string
  author: string | null
  url: string
  source_date: string | null
  source_name: string | null
}

function articleLink(url: string) {
  try {
    const parsed = new URL(url)
    return ['http:', 'https:'].includes(parsed.protocol) ? parsed.href : undefined
  } catch {
    return undefined
  }
}

function articleDate(value: string | null) {
  if (!value) return null
  const date = new Date(value)
  return Number.isNaN(date.getTime())
    ? null
    : date.toLocaleDateString(undefined, { month: 'short', day: 'numeric' })
}

function NewsPaper() {
  const [today] = useState(() => new Date())
  const [articles, setArticles] = useState<Article[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [edition, setEdition] = useState(0)

  useEffect(() => {
    let active = true

    async function loadArticles() {
      setLoading(true)
      setError(null)
      try {
        const data = await fetchToday()
        if (!Array.isArray(data)) throw new Error('Unexpected article response')
        if (active) setArticles(data)
      } catch (err) {
        if (active) {
          setError(err instanceof Error ? err.message : 'Could not load articles')
        }
      } finally {
        if (active) setLoading(false)
      }
    }

    void loadArticles()
    return () => { active = false }
  }, [edition])

  return (
    <div className="newspaper">
      <header className="newspaper-header">
        <div className="edition-line">
          <span>Your daily reading list</span>
          <time dateTime={today.toLocaleDateString('en-CA')}>
            {today.toLocaleDateString(undefined, {
              weekday: 'long', month: 'long', day: 'numeric', year: 'numeric',
            })}
          </time>
        </div>
        <h1>The Daily Brief</h1>
        <p>A little perspective. A few good reads.</p>
      </header>

      <main id="headlines" aria-busy={loading}>
        <div className="section-heading">
          <div>
            <h2>Today’s headlines</h2>
            <p aria-live="polite">
              {loading ? 'Gathering the latest stories…' : error ? 'Feed unavailable' : `${articles.length} ${articles.length === 1 ? 'story' : 'stories'} to explore`}
            </p>
          </div>
          <button
            className="refresh-button"
            type="button"
            disabled={loading}
            onClick={() => setEdition((value) => value + 1)}
          >
            {loading ? 'Loading…' : 'Refresh'}
          </button>
        </div>

        {loading ? (
          <div className="newspaper-content" aria-hidden="true">
            {[0, 1, 2, 3].map((item) => (
              <div className="news-article skeleton" key={item}>
                <div className="skeleton-label" />
                <div className="skeleton-title" />
                <div className="skeleton-line" />
              </div>
            ))}
          </div>
        ) : error ? (
          <div className="feed-message" role="alert">
            <h3>The headlines couldn’t make it here.</h3>
            <p>{error}. Check that the API is running at localhost:6767, then try again.</p>
            <button type="button" className="refresh-button" onClick={() => setEdition((value) => value + 1)}>
              Try again
            </button>
          </div>
        ) : articles.length === 0 ? (
          <div className="feed-message" role="status">
            <h3>A quiet edition.</h3>
            <p>No unread articles right now. Check back later for fresh stories.</p>
          </div>
        ) : (
          <div className="newspaper-content">
            {articles.map((article) => {
              const href = articleLink(article.url)
              const date = articleDate(article.source_date)

              return (
                <article key={article.id} className="news-article">
                  <div className="article-meta">
                    <span>{article.source_name || 'From the wire'}</span>
                    {date && <time dateTime={article.source_date!}>{date}</time>}
                  </div>
                  <h3>
                    {href ? <a href={href} target="_blank" rel="noopener noreferrer">{article.title}</a> : article.title}
                  </h3>
                  <div className="article-footer">
                    {article.author && <span className="article-author">By {article.author}</span>}
                    {href && (
                      <a className="read-link" href={href} target="_blank" rel="noopener noreferrer" aria-label={`Read ${article.title} (opens in a new tab)`}>
                        Read story <span aria-hidden="true">↗</span>
                      </a>
                    )}
                  </div>
                </article>
              )
            })}
          </div>
        )}
      </main>

      <footer className="newspaper-footer">
        <span>The Daily Brief</span>
        <span>Good stories, less noise.</span>
      </footer>
    </div>
  )
}

export default NewsPaper
