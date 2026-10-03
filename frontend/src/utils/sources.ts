export interface SourceFields {
  name: string
  url: string
  is_active: boolean
}

export interface Source extends SourceFields {
  id: number
}

export type SourceSortKey = 'name' | 'url' | 'status'

export interface SourceSort {
  key: SourceSortKey
  direction: 'asc' | 'desc'
}

export function filterAndSortSources(sources: Source[], search: string, sort: SourceSort): Source[] {
  const query = search.trim().toLocaleLowerCase()
  return sources
    .filter((source) => source.name.toLocaleLowerCase().includes(query) || source.url.toLocaleLowerCase().includes(query))
    .sort((a, b) => {
      const nameOrder = a.name.localeCompare(b.name, undefined, { sensitivity: 'base', numeric: true })
      const primaryOrder = sort.key === 'status'
        ? Number(b.is_active) - Number(a.is_active)
        : sort.key === 'url'
          ? a.url.localeCompare(b.url, undefined, { sensitivity: 'base', numeric: true })
          : nameOrder
      return (sort.direction === 'asc' ? primaryOrder : -primaryOrder) || nameOrder || a.id - b.id
    })
}

export function normalizeSource(source: SourceFields): SourceFields {
  return { name: source.name.trim(), url: source.url.trim(), is_active: source.is_active }
}

export function validateSource(source: SourceFields): string | null {
  if (!source.name.trim()) return 'Enter a source name.'
  try {
    const url = new URL(source.url.trim())
    if (!['http:', 'https:'].includes(url.protocol) || !url.hostname) throw new Error('Invalid URL')
  } catch {
    return 'Enter a complete HTTP or HTTPS feed URL.'
  }
  return null
}

export function changedSources(original: Source[], draft: Source[]): Source[] {
  return draft.filter((source) => {
    const previous = original.find((item) => item.id === source.id)
    const normalized = normalizeSource(source)
    return previous && (previous.name !== normalized.name || previous.url !== normalized.url || previous.is_active !== normalized.is_active)
  })
}
