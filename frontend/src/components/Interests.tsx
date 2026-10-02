import { useEffect, useState } from 'react'
import { changeInterests, fetchInterests } from '../../api'
import type { InterestUpdate } from '../../api'

interface Interest {
  id: number
  keyword: string
  weight: number
  is_main: boolean
  is_active: boolean
}

function Interests() {
  const [interests, setInterests] = useState<Interest[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [refresh, setRefresh] = useState(0)
  const [draft, setDraft] = useState<Interest[] | null>(null)
  const [saving, setSaving] = useState(false)
  const [saveError, setSaveError] = useState<string | null>(null)
  const [notice, setNotice] = useState<string | null>(null)

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
          <h2>Your interests</h2>
          <p aria-live="polite">
            {loading ? 'Gathering your interests…' : error ? 'Interests unavailable' : `${interests.length} ${interests.length === 1 ? 'interest' : 'interests'} · ${activeCount} active`}
          </p>
        </div>
        <div className="interests-actions">
          <button
            className="refresh-button"
            type="button"
            disabled={loading || saving}
            onClick={editing ? cancelEditing : () => setRefresh((value) => value + 1)}
          >
            {editing ? 'Cancel' : loading ? 'Loading…' : 'Refresh'}
          </button>
          <button
            className={`refresh-button${editing ? ' interests-save-button' : ''}`}
            type="button"
            disabled={loading || saving || !!error || interests.length === 0 || (editing && changedInterests.length === 0)}
            onClick={editing ? () => void saveChanges() : startEditing}
          >
            {editing ? saving ? 'Saving…' : 'Save changes' : 'Edit interests'}
          </button>
        </div>
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
          <p
            className={`interests-feedback${saveError ? ' is-error' : notice ? ' is-success' : ''}`}
            role={saveError ? 'alert' : 'status'}
            title={feedback}
          >
            {feedback}
          </p>
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
                  <th scope="col">Keyword</th>
                  <th scope="col">Role</th>
                  <th scope="col">Status</th>
                </tr>
              </thead>
              <tbody>
                {displayedInterests.map((interest) => (
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
