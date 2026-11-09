const LAST_UPDATED = '2026-11-09'
const ISSUES_URL = 'https://github.com/aeron-cap/news-aggregate-go/issues'
const GITHUB_PRIVACY_URL = 'https://docs.github.com/en/site-policy/privacy-policies/github-general-privacy-statement'

function Legal() {
  return (
    <main id="legal">
      <div className="section-heading">
        <div>
          <h2>Legal &amp; disclaimer</h2>
          <p>
            Last updated{' '}
            <time dateTime={LAST_UPDATED}>
              {new Date(`${LAST_UPDATED}T00:00:00`).toLocaleDateString(undefined, { month: 'long', day: 'numeric', year: 'numeric' })}
            </time>
          </p>
        </div>
      </div>

      <article className="legal-content">
        <section>
          <h3>About</h3>
          <p>A free, non-commercial project that links to articles from public RSS and Atom feeds.</p>
        </section>

        <section>
          <h3>Content</h3>
          <p>
            Articles and trademarks belong to their publishers. We only show headlines and links, never full
            articles. Listing a source doesn’t mean we’re affiliated with it.
          </p>
        </section>

        <section>
          <h3>Removal</h3>
          <p>
            Own a feed and want it removed?{' '}
            <a href={ISSUES_URL} target="_blank" rel="noreferrer">Open a GitHub issue</a>.
          </p>
        </section>

        <section>
          <h3>Your sources</h3>
          <p>You’re responsible for any feeds you add.</p>
        </section>

        <section>
          <h3>No warranty</h3>
          <p>
            Provided as is. We aren’t responsible for errors, linked websites, or any loss from using this site.
          </p>
        </section>

        <section>
          <h3>Privacy</h3>
          <p>
            No accounts and no tracking. One cookie keeps your anonymous settings, which are deleted after 15 days
            without a visit. The footer’s Star
            button loads from GitHub, which has its own{' '}
            <a href={GITHUB_PRIVACY_URL} target="_blank" rel="noreferrer">privacy policy</a>.
          </p>
        </section>

        <section>
          <h3>Changes</h3>
          <p>This page may change. Check the date above.</p>
        </section>
      </article>
    </main>
  )
}

export default Legal
