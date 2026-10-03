import { useEffect, useState } from 'react'
import { fetchToday, markArticleAsRead } from '../../api'

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

function ArticleCard({ article, onRead }: { article: Article; onRead: (id: number) => Promise<void> }) {
  const [showReadPrompt, setShowReadPrompt] = useState(false)
  const [markingRead, setMarkingRead] = useState(false)
  const [readError, setReadError] = useState<string | null>(null)
  const href = articleLink(article.url)
  const date = articleDate(article.source_date)

  function openArticle() {
    setShowReadPrompt(true)
  }

  async function confirmRead() {
    if (markingRead) return
    setMarkingRead(true)
    setReadError(null)
    try {
      await onRead(article.id)
    } catch (err) {
      setReadError(err instanceof Error ? err.message : 'Could not mark article as read')
    } finally {
      setMarkingRead(false)
    }
  }

  function keepUnread() {
    setShowReadPrompt(false)
    setReadError(null)
  }

  return (
    <article className="news-article" aria-busy={markingRead}>
      <div className="article-meta">
        <span>{article.source_name || 'From the wire'}</span>
        {date && <time dateTime={article.source_date!}>{date}</time>}
      </div>
      <h3>
        {href ? (
          <a
            href={href}
            target="_blank"
            rel="noopener noreferrer"
            onClick={openArticle}
            onAuxClick={(event) => { if (event.button === 1) openArticle() }}
          >
            {article.title}
          </a>
        ) : article.title}
      </h3>
      <div className="article-footer">
        {article.author && <span className="article-author">By {article.author}</span>}
        {href && (
          <div className="article-actions">
            <div
              className="article-read-prompt"
              role="group"
              aria-label={`Did you read ${article.title}?`}
              aria-hidden={!showReadPrompt}
              style={{ visibility: showReadPrompt ? 'visible' : 'hidden' }}
            >
              <span className="article-read-question" aria-live="polite">
                {markingRead ? 'Saving…' : 'Did you read it?'}
              </span>
              <button
                className="article-read-choice is-confirm"
                type="button"
                aria-label={`Yes, mark ${article.title} as read`}
                title="Yes, mark as read"
                disabled={!showReadPrompt || markingRead}
                onClick={() => void confirmRead()}
              >
                <span aria-hidden="true">✓</span>
              </button>
              <button
                className="article-read-choice"
                type="button"
                aria-label={`No, keep ${article.title} unread`}
                title="No, keep unread"
                disabled={!showReadPrompt || markingRead}
                onClick={keepUnread}
              >
                <span aria-hidden="true">×</span>
              </button>
            </div>
            <a
              className="read-link"
              href={href}
              target="_blank"
              rel="noopener noreferrer"
              aria-label={`Read ${article.title} (opens in a new tab)`}
              onClick={openArticle}
              onAuxClick={(event) => { if (event.button === 1) openArticle() }}
            >
              Read story <span aria-hidden="true">↗</span>
            </a>
          </div>
        )}
      </div>
      {readError && <p className="article-read-error" role="alert">{readError}. Try again.</p>}
    </article>
  )
}

function NewsPaper() {
  const [articles, setArticles] = useState<Article[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [edition, setEdition] = useState(0)
  const [pendingReads, setPendingReads] = useState(0)

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

  async function confirmArticleRead(id: number) {
    setPendingReads((count) => count + 1)
    try {
      await markArticleAsRead(id)
      setArticles((current) => current.filter((article) => article.id !== id))
    } finally {
      setPendingReads((count) => count - 1)
    }
  }

  return (
      <main id="feed" aria-busy={loading}>
        <div className="section-heading">
          <div>
            <h2>Your feed</h2>
            <p aria-live="polite">
              {loading ? 'Gathering the latest stories…' : error ? 'Feed unavailable' : `${articles.length} ${articles.length === 1 ? 'story' : 'stories'} to explore`}
            </p>
          </div>
          <button
            className="refresh-button"
            type="button"
            disabled={loading || pendingReads > 0}
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
            <h3>Your feed couldn’t make it here.</h3>
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
            {articles.map((article) => (
              <ArticleCard key={article.id} article={article} onRead={confirmArticleRead} />
            ))}
          </div>
        )}
      </main>
  )
}

export default NewsPaper
