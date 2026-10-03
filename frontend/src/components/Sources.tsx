import { useEffect, useState } from 'react'
import { fetchSources, insertSources, updateSources } from '../../api'
import SearchInput from './SearchInput'
import { changedSources, filterAndSortSources, normalizeSource, validateSource } from '../utils/sources'
import type { Source, SourceFields, SourceSort, SourceSortKey } from '../utils/sources'

function Sources() {
  const [sources, setSources] = useState<Source[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [refresh, setRefresh] = useState(0)
  const [draft, setDraft] = useState<Source[] | null>(null)
  const [newSource, setNewSource] = useState<SourceFields | null>(null)
  const [saving, setSaving] = useState(false)
  const [saveError, setSaveError] = useState<string | null>(null)
  const [notice, setNotice] = useState<string | null>(null)
  const [search, setSearch] = useState('')
  const [sort, setSort] = useState<SourceSort>({ key: 'name', direction: 'asc' })

  useEffect(() => {
    let active = true
    async function loadSources() {
      setLoading(true)
      setError(null)
      try {
        const data = await fetchSources()
        if (!Array.isArray(data)) throw new Error('Unexpected sources response')
        if (active) setSources(data)
      } catch (err) {
        if (active) setError(err instanceof Error ? err.message : 'Could not load sources')
      } finally {
        if (active) setLoading(false)
      }
    }
    void loadSources()
    return () => { active = false }
  }, [refresh])

  const editing = draft !== null
  const displayedSources = draft ?? sources
  const visibleSources = filterAndSortSources(displayedSources, search, sort)
  const activeCount = displayedSources.filter((source) => source.is_active).length
  const changes = changedSources(sources, draft ?? [])
  const feedback = saveError
    ? `${saveError} Your edits are still here.`
    : notice ?? (saving
      ? 'Saving sources…'
      : editing
        ? `${changes.length} ${changes.length === 1 ? 'source' : 'sources'} changed · Edit details or status, then save.`
        : 'Active sources are used the next time your feed is built.')

  function changeSort(key: SourceSortKey) {
    setSort((current) => ({ key, direction: current.key === key && current.direction === 'asc' ? 'desc' : 'asc' }))
  }

  function clearFeedback() {
    setSaveError(null)
    setNotice(null)
  }

  function changeSource(id: number, update: Partial<SourceFields>) {
    setDraft((current) => current?.map((source) => source.id === id ? { ...source, ...update } : source) ?? null)
    clearFeedback()
  }

  function validationError(items: SourceFields[]) {
    for (const source of items) {
      const message = validateSource(source)
      if (message) return message
    }
    const urls = items.map((source) => source.url.trim())
    return new Set(urls).size !== urls.length ? 'Each source must have a unique feed URL.' : null
  }

  async function saveChanges() {
    if (!draft || saving || changes.length === 0) return
    const message = validationError(draft)
    if (message) { setSaveError(message); return }
    const updates: Record<number, SourceFields> = {}
    for (const source of changes) updates[source.id] = normalizeSource(source)
    setSaving(true)
    clearFeedback()
    try {
      await updateSources(updates)
      setSources(draft.map((source) => ({ id: source.id, ...normalizeSource(source) })))
      setDraft(null)
      setNotice('Sources saved. Applies the next time your feed is built.')
    } catch (err) {
      setSaveError(err instanceof Error ? err.message : 'Could not save sources')
    } finally {
      setSaving(false)
    }
  }

  async function addSource() {
    if (!newSource || saving) return
    const message = validationError([...sources, newSource])
    if (message) { setSaveError(message); return }
    setSaving(true)
    clearFeedback()
    try {
      await insertSources([normalizeSource(newSource)])
      setNewSource(null)
      setNotice('Source added. Applies the next time your feed is built.')
      setRefresh((value) => value + 1)
    } catch (err) {
      setSaveError(err instanceof Error ? err.message : 'Could not add source')
    } finally {
      setSaving(false)
    }
  }

  return (
    <main id="sources" aria-busy={loading || saving}>
      <div className="section-heading">
        <div>
          <div className="interests-title sources-title">
            <h2>Your sources</h2>
            <button
              className="refresh-button interests-edit-button"
              type="button"
              aria-label={editing ? 'Cancel source edits' : 'Edit sources'}
              disabled={loading || saving || !!error || !!newSource || sources.length === 0}
              onClick={() => {
                setDraft(editing ? null : sources.map((source) => ({ ...source })))
                clearFeedback()
              }}
            >
              {editing ? 'Cancel' : 'Edit'}
            </button>
          </div>
          <p aria-live="polite">
            {loading ? 'Gathering your sources…' : error ? 'Sources unavailable' : `${sources.length} ${sources.length === 1 ? 'source' : 'sources'} · ${activeCount} active`}
          </p>
        </div>
        <div className="sources-actions">
          <button
            className="refresh-button"
            type="button"
            disabled={loading || saving || editing || !!newSource || !!error}
            onClick={() => { setNewSource({ name: '', url: '', is_active: true }); clearFeedback() }}
          >Add source</button>
          <button
            className="refresh-button"
            type="button"
            disabled={loading || saving || editing || !!newSource}
            onClick={() => { clearFeedback(); setRefresh((value) => value + 1) }}
          >{loading ? 'Loading…' : 'Refresh'}</button>
        </div>
      </div>

      {loading ? (
        <div className="interests-loading" aria-hidden="true">
          {[0, 1, 2, 3].map((item) => <div className="interest-skeleton" key={item}><span /><span /></div>)}
        </div>
      ) : error ? (
        <div className="feed-message" role="alert">
          <h3>Your sources couldn’t make it here.</h3>
          <p>{error}. Check that the API is running at localhost:6767, then try again.</p>
          <button className="refresh-button" type="button" onClick={() => setRefresh((value) => value + 1)}>Try again</button>
        </div>
      ) : (
        <>
          <div className="interests-feedback-row sources-feedback-row">
            <p className={`interests-feedback${saveError ? ' is-error' : notice ? ' is-success' : ''}`} role={saveError ? 'alert' : 'status'} title={feedback}>
              {feedback}
            </p>
            <button
              className="refresh-button interests-save-button"
              type="button"
              style={{ visibility: editing ? 'visible' : 'hidden' }}
              disabled={!editing || saving || changes.length === 0}
              onClick={() => void saveChanges()}
            >{saving && editing ? 'Saving…' : 'Save changes'}</button>
          </div>

          {newSource && (
            <form className="source-form" onSubmit={(event) => { event.preventDefault(); void addSource() }}>
              <h3>Add a source</h3>
              <div className="source-form-fields">
                <label>Name
                  <input required value={newSource.name} disabled={saving} onChange={(event) => { setNewSource({ ...newSource, name: event.target.value }); clearFeedback() }} placeholder="Source name" />
                </label>
                <label>Feed URL
                  <input required type="url" value={newSource.url} disabled={saving} onChange={(event) => { setNewSource({ ...newSource, url: event.target.value }); clearFeedback() }} placeholder="https://example.com/rss" />
                </label>
              </div>
              <p className="source-form-hint">Use an RSS or Atom feed URL, not the website homepage.</p>
              <div className="source-form-actions">
                <label className="source-active-label">
                  <input type="checkbox" checked={newSource.is_active} disabled={saving} onChange={(event) => { setNewSource({ ...newSource, is_active: event.target.checked }); clearFeedback() }} />
                  Active
                </label>
                <button className="refresh-button" type="button" disabled={saving} onClick={() => { setNewSource(null); clearFeedback() }}>Cancel</button>
                <button className="refresh-button interests-save-button" type="submit" disabled={saving}>{saving ? 'Adding…' : 'Add source'}</button>
              </div>
            </form>
          )}

          {sources.length === 0 ? (
            <div className="feed-message" role="status">
              <h3>A blank slate.</h3>
              <p>No sources have been added yet. Add an RSS or Atom feed to get started.</p>
            </div>
          ) : (
            <>
              <div className="interests-search-row">
                <SearchInput value={search} onChange={setSearch} label="Search sources" />
                <p className="interests-search-count" role="status">{visibleSources.length} of {sources.length}</p>
              </div>
              <div className="interests-table-wrapper" role="region" aria-label="All sources" tabIndex={0}>
                <table className="interests-table sources-table">
                  <caption>All sources</caption>
                  <colgroup><col className="source-name-column" /><col /><col className="interest-status-column" /></colgroup>
                  <thead>
                    <tr>
                      {(['name', 'url', 'status'] as const).map((key) => (
                        <th key={key} scope="col" aria-sort={sort.key === key ? sort.direction === 'asc' ? 'ascending' : 'descending' : undefined}>
                          <button className="interest-sort-button" type="button" onClick={() => changeSort(key)} aria-label={`Sort by ${key}`}>
                            {key === 'url' ? 'Feed URL' : key === 'name' ? 'Name' : 'Status'}
                            <span aria-hidden="true">{sort.key === key ? sort.direction === 'asc' ? '↑' : '↓' : '↕'}</span>
                          </button>
                        </th>
                      ))}
                    </tr>
                  </thead>
                  <tbody>
                    {visibleSources.length === 0 && <tr><td colSpan={3} className="interests-no-results">No sources match “{search.trim()}”.</td></tr>}
                    {visibleSources.map((source) => (
                      <tr key={source.id}>
                        <th scope="row">
                          {editing ? <input className="source-input" aria-label={`Source name: ${source.name}`} value={source.name} disabled={saving} onChange={(event) => changeSource(source.id, { name: event.target.value })} /> : source.name}
                        </th>
                        <td>
                          {editing ? <input className="source-input" type="url" aria-label={`Feed URL: ${source.name}`} value={source.url} disabled={saving} onChange={(event) => changeSource(source.id, { url: event.target.value })} /> : (
                            <a className="source-url" href={source.url} target="_blank" rel="noreferrer">{source.url}</a>
                          )}
                        </td>
                        <td>
                          {editing ? (
                            <button className={`interest-value interest-status interest-toggle${source.is_active ? ' is-active' : ''}`} type="button" aria-label={`Active source: ${source.name}`} aria-pressed={source.is_active} disabled={saving} onClick={() => changeSource(source.id, { is_active: !source.is_active })}>
                              {source.is_active ? 'Active' : 'Inactive'}
                            </button>
                          ) : <span className={`interest-value interest-status${source.is_active ? ' is-active' : ''}`}>{source.is_active ? 'Active' : 'Inactive'}</span>}
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            </>
          )}
        </>
      )}
    </main>
  )
}

export default Sources
