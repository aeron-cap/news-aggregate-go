import { useEffect, useState } from 'react'
import Interests from './components/Interests'
import NewsPaper from './components/NewsPaper'
import './styles/NewsPaper.css'
import './styles/Interests.css'

function App() {
  const [today] = useState(() => new Date())
  const [page, setPage] = useState(() => window.location.hash === '#/interests' ? 'interests' : 'headlines')

  useEffect(() => {
    function updatePage() {
      setPage(window.location.hash === '#/interests' ? 'interests' : 'headlines')
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
        <a href="#/" aria-current={page === 'headlines' ? 'page' : undefined}>Headlines</a>
        <a href="#/interests" aria-current={page === 'interests' ? 'page' : undefined}>Interests</a>
      </nav>

      {page === 'interests' ? <Interests /> : <NewsPaper />}

      <footer className="newspaper-footer">
        <span>The Daily Brief</span>
        <span>Good stories, less noise.</span>
      </footer>
    </div>
  )
}

export default App
