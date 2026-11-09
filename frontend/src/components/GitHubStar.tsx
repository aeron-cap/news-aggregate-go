import { useEffect } from 'react'

const SCRIPT_SRC = 'https://buttons.github.io/buttons.js'
const REPO_URL = 'https://github.com/aeron-cap/news-aggregate-go'

// GitHub's official button. Its script swaps the link below for an iframe served by GitHub.
function GitHubStar() {
  useEffect(() => {
    if (document.querySelector(`script[src="${SCRIPT_SRC}"]`)) return
    const script = document.createElement('script')
    script.src = SCRIPT_SRC
    script.async = true
    script.defer = true
    document.body.appendChild(script)
  }, [])

  return (
    <span className="github-star">
      <a
        className="github-button"
        href={REPO_URL}
        data-icon="octicon-star"
        data-show-count="true"
        aria-label="Star aeron-cap/news-aggregate-go on GitHub"
      >Star</a>
    </span>
  )
}

export default GitHubStar
