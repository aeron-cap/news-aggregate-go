import { useEffect, useState } from 'react'
import { changeInterests, fetchInterests } from '../../api'
import type { InterestUpdate } from '../../api'
import SearchInput from './SearchInput'
import { filterAndSortInterests } from '../utils/interests'
import type { Interest, InterestSort, InterestSortKey } from '../utils/interests'

function Interests() {
  const [interests, setInterests] = useState<Interest[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [refresh, setRefresh] = useState(0)
  const [draft, setDraft] = useState<Interest[] | null>(null)
  const [saving, setSaving] = useState(false)
  const [saveError, setSaveError] = useState<string | null>(null)
  const [notice, setNotice] = useState<string | null>(null)
  const [search, setSearch] = useState('')
  const [sort, setSort] = useState<InterestSort>({ key: 'name', direction: 'asc' })

  useEffect(() => {
    let active = true

    async function loadInterests() {
      setLoading(true)
      setError(null)
      setNotice(null)
      try {
        const data = await fetchInterests()
        if (!Array.isArray(data)) throw new Error('Unexpected interests response')
        if (active) setInterests(data)
      } catch (err) {
        if (active) {
          setError(err instanceof Error ? err.message : 'Could not load interests')
        }
      } finally {
        if (active) setLoading(false)
      }
    }

    void loadInterests()
    return () => { active = false }
  }, [refresh])

  const editing = draft !== null
  const displayedInterests = draft ?? interests
  const visibleInterests = filterAndSortInterests(displayedInterests, search, sort)
  const activeCount = displayedInterests.filter((interest) => interest.is_active).length
  const changedInterests = (draft ?? []).filter((interest) => {
    const original = interests.find((item) => item.id === interest.id)
    return original && (original.is_main !== interest.is_main || original.is_active !== interest.is_active)
  })
  const feedback = saveError
    ? `${saveError}. Your edits are still here; try saving again.`
    : notice ?? (saving
      ? 'Saving preferences…'
      : editing
        ? `${changedInterests.length} ${changedInterests.length === 1 ? 'interest' : 'interests'} changed · Toggle role or status, then save.`
        : 'Main interests guide your feed; supporting interests add related topics.')

  function changeSort(key: InterestSortKey) {
    setSort((current) => ({
      key,
      direction: current.key === key && current.direction === 'asc' ? 'desc' : 'asc',
    }))
  }

  function sortDirection(key: InterestSortKey) {
    return sort.key === key ? sort.direction === 'asc' ? 'ascending' : 'descending' : undefined
  }

  function sortIndicator(key: InterestSortKey) {
    return sort.key === key ? sort.direction === 'asc' ? '↑' : '↓' : '↕'
  }

  function startEditing() {
    setDraft(interests.map((interest) => ({ ...interest })))
    setSaveError(null)
    setNotice(null)
  }

  function cancelEditing() {
    setDraft(null)
    setSaveError(null)
  }

  function toggleInterest(id: number, field: 'is_main' | 'is_active') {
    setDraft((current) => current?.map((interest) => (
      interest.id === id ? { ...interest, [field]: !interest[field] } : interest
    )) ?? null)
    setSaveError(null)
  }

  async function saveChanges() {
    if (!draft || saving || changedInterests.length === 0) return

    const updates: Record<number, InterestUpdate> = {}
    for (const interest of changedInterests) {
      updates[interest.id] = { isActive: interest.is_active, isMain: interest.is_main }
    }

    setSaving(true)
    setSaveError(null)
    try {
      await changeInterests(updates)
      setInterests(draft)
      setDraft(null)
      setNotice('Interests saved. Applies when new articles are scored.')
    } catch (err) {
      setSaveError(err instanceof Error ? err.message : 'Could not save interests')
    } finally {
      setSaving(false)
    }
  }

  return (
    <main id="interests" aria-busy={loading || saving}>
      <div className="section-heading">
        <div>
          <div className="interests-title">
            <h2>Your interests</h2>
            <button
              className="refresh-button interests-edit-button"
              type="button"
              aria-label={editing ? 'Cancel interest edits' : 'Edit interests'}
              disabled={loading || saving || !!error || interests.length === 0}
              onClick={editing ? cancelEditing : startEditing}
            >
              {editing ? 'Cancel' : 'Edit'}
            </button>
          </div>
          <p aria-live="polite">
            {loading ? 'Gathering your interests…' : error ? 'Interests unavailable' : `${interests.length} ${interests.length === 1 ? 'interest' : 'interests'} · ${activeCount} active`}
          </p>
        </div>
        <button
          className="refresh-button"
          type="button"
          disabled={loading || saving || editing}
          onClick={() => setRefresh((value) => value + 1)}
        >
          {loading ? 'Loading…' : 'Refresh'}
        </button>
      </div>

      {loading ? (
        <div className="interests-loading" aria-hidden="true">
          {[0, 1, 2, 3].map((item) => (
            <div className="interest-skeleton" key={item}>
              <span />
              <span />
            </div>
          ))}
        </div>
      ) : error ? (
        <div className="feed-message" role="alert">
          <h3>Your interests couldn’t make it here.</h3>
          <p>{error}. Check that the API is running at localhost:6767, then try again.</p>
          <button className="refresh-button" type="button" onClick={() => setRefresh((value) => value + 1)}>
            Try again
          </button>
        </div>
      ) : interests.length === 0 ? (
        <div className="feed-message" role="status">
          <h3>A blank slate.</h3>
          <p>No interests have been added yet.</p>
        </div>
      ) : (
        <>
          <div className="interests-feedback-row">
            <p
              className={`interests-feedback${saveError ? ' is-error' : notice ? ' is-success' : ''}`}
              role={saveError ? 'alert' : 'status'}
              title={feedback}
            >
              {feedback}
            </p>
            <button
              className="refresh-button interests-save-button"
              type="button"
              style={{ visibility: editing ? 'visible' : 'hidden' }}
              disabled={!editing || saving || changedInterests.length === 0}
              onClick={() => void saveChanges()}
            >
              {saving ? 'Saving…' : 'Save changes'}
            </button>
          </div>
          <div className="interests-search-row">
            <SearchInput value={search} onChange={setSearch} label="Search interests" />
            <p className="interests-search-count" role="status">
              {visibleInterests.length} of {interests.length}
            </p>
          </div>
          <div className="interests-table-wrapper" role="region" aria-label="All interests" tabIndex={0}>
            <table className="interests-table">
              <caption>All interests</caption>
              <colgroup>
                <col />
                <col className="interest-role-column" />
                <col className="interest-status-column" />
              </colgroup>
              <thead>
                <tr>
                  <th scope="col" aria-sort={sortDirection('name')}>
                    <button className="interest-sort-button" type="button" onClick={() => changeSort('name')} aria-label="Sort by name">
                      Name <span aria-hidden="true">{sortIndicator('name')}</span>
                    </button>
                  </th>
                  <th scope="col" aria-sort={sortDirection('role')}>
                    <button className="interest-sort-button" type="button" onClick={() => changeSort('role')} aria-label="Sort by role">
                      Role <span aria-hidden="true">{sortIndicator('role')}</span>
                    </button>
                  </th>
                  <th scope="col" aria-sort={sortDirection('status')}>
                    <button className="interest-sort-button" type="button" onClick={() => changeSort('status')} aria-label="Sort by status">
                      Status <span aria-hidden="true">{sortIndicator('status')}</span>
                    </button>
                  </th>
                </tr>
              </thead>
              <tbody>
                {visibleInterests.length === 0 && (
                  <tr>
                    <td colSpan={3} className="interests-no-results">No interests match “{search.trim()}”.</td>
                  </tr>
                )}
                {visibleInterests.map((interest) => (
                  <tr key={interest.id}>
                    <th scope="row">{interest.keyword}</th>
                    <td>
                      {editing ? (
                        <button
                          className="interest-value interest-role interest-toggle"
                          type="button"
                          aria-label={`Main interest: ${interest.keyword}`}
                          aria-pressed={interest.is_main}
                          disabled={saving}
                          onClick={() => toggleInterest(interest.id, 'is_main')}
                        >
                          {interest.is_main ? 'Main' : 'Supporting'}
                        </button>
                      ) : (
                        <span className="interest-value interest-role">
                          {interest.is_main ? 'Main' : 'Supporting'}
                        </span>
                      )}
                    </td>
                    <td>
                      {editing ? (
                        <button
                          className={`interest-value interest-status interest-toggle${interest.is_active ? ' is-active' : ''}`}
                          type="button"
                          aria-label={`Active interest: ${interest.keyword}`}
                          aria-pressed={interest.is_active}
                          disabled={saving}
                          onClick={() => toggleInterest(interest.id, 'is_active')}
                        >
                          {interest.is_active ? 'Active' : 'Inactive'}
                        </button>
                      ) : (
                        <span className={`interest-value interest-status${interest.is_active ? ' is-active' : ''}`}>
                          {interest.is_active ? 'Active' : 'Inactive'}
                        </span>
                      )}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </>
      )}
    </main>
  )
}

export default Interests
