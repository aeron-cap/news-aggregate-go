export interface Interest {
  id: number
  keyword: string
  weight: number
  is_main: boolean
  is_active: boolean
}

export type InterestSortKey = 'name' | 'role' | 'status'

export interface InterestSort {
  key: InterestSortKey
  direction: 'asc' | 'desc'
}

export function filterAndSortInterests(interests: Interest[], search: string, sort: InterestSort): Interest[] {
  const query = search.trim().toLocaleLowerCase()
  return interests
    .filter((interest) => interest.keyword.toLocaleLowerCase().includes(query))
    .sort((a, b) => {
      const nameOrder = a.keyword.localeCompare(b.keyword, undefined, { sensitivity: 'base', numeric: true })
      const primaryOrder = sort.key === 'role'
        ? Number(b.is_main) - Number(a.is_main)
        : sort.key === 'status'
          ? Number(b.is_active) - Number(a.is_active)
          : nameOrder
      return (sort.direction === 'asc' ? primaryOrder : -primaryOrder) || nameOrder || a.id - b.id
    })
}
