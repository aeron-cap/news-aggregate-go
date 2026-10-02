import { useEffect, useState } from 'react'
import Interests from './components/Interests'
import NewsPaper from './components/NewsPaper'
import './styles/NewsPaper.css'
import './styles/Interests.css'

type Page = 'feed' | 'interests'

function currentPage(): Page {
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
        <h1>The Daily Brief</h1>
        <p>A little perspective. A few good reads.</p>
      </header>

      <nav className="page-navigation" aria-label="Main navigation">
        <a href="#/" aria-current={page === 'feed' ? 'page' : undefined}>Feed</a>
        <a href="#/interests" aria-current={page === 'interests' ? 'page' : undefined}>Interests</a>
      </nav>

      <div hidden={page !== 'feed'}>
        {visitedPages.has('feed') && <NewsPaper />}
      </div>
      <div hidden={page !== 'interests'}>
        {visitedPages.has('interests') && <Interests />}
      </div>

      <footer className="newspaper-footer">
        <span>The Daily Brief</span>
        <span>Good stories, less noise.</span>
      </footer>
    </div>
  )
}

export default App
