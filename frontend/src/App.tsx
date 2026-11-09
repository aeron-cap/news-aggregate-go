import { useEffect, useState } from 'react'
import Interests from './components/Interests'
import NewsPaper from './components/NewsPaper'
import Sources from './components/Sources'
import Legal from './components/Legal'
import GitHubStar from './components/GitHubStar'
import './styles/NewsPaper.css'
import './styles/Interests.css'
import './styles/Sources.css'
import './styles/Legal.css'

type Page = 'feed' | 'interests' | 'sources' | 'legal'

function currentPage(): Page {
  if (window.location.hash === '#/sources') return 'sources'
  if (window.location.hash === '#/legal') return 'legal'
  return window.location.hash === '#/interests' ? 'interests' : 'feed'
}

function App() {
  const [today] = useState(() => new Date())
  const [page, setPage] = useState(currentPage)
  const [visitedPages, setVisitedPages] = useState(() => new Set<Page>([page]))

  useEffect(() => {
    function updatePage() {
      const nextPage = currentPage()
      setPage(nextPage)
      setVisitedPages((visited) => visited.has(nextPage) ? visited : new Set([...visited, nextPage]))
    }

    window.addEventListener('hashchange', updatePage)
    return () => window.removeEventListener('hashchange', updatePage)
  }, [])

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
        <h1>PocketNews</h1>
        <p>A little perspective. A few good reads.</p>
      </header>

      <nav className="page-navigation" aria-label="Main navigation">
        <a href="#/" aria-current={page === 'feed' ? 'page' : undefined}>Feed</a>
        <a href="#/interests" aria-current={page === 'interests' ? 'page' : undefined}>Interests</a>
        <a href="#/sources" aria-current={page === 'sources' ? 'page' : undefined}>Sources</a>
        <a href="#/legal" aria-current={page === 'legal' ? 'page' : undefined}>Legal</a>
      </nav>

      <div hidden={page !== 'feed'}>
        {visitedPages.has('feed') && <NewsPaper />}
      </div>
      <div hidden={page !== 'interests'}>
        {visitedPages.has('interests') && <Interests />}
      </div>
      <div hidden={page !== 'sources'}>
        {visitedPages.has('sources') && <Sources />}
      </div>
      <div hidden={page !== 'legal'}>
        {visitedPages.has('legal') && <Legal />}
      </div>

      <footer className="newspaper-footer">
        <span className="footer-brand">
          PocketNews
          <GitHubStar />
        </span>
        <span>Good stories, less noise. · <a href="#/legal">Legal &amp; disclaimer</a></span>
      </footer>
    </div>
  )
}

export default App
