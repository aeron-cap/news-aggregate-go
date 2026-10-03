import '../styles/SearchInput.css'

interface SearchInputProps {
  value: string
  onChange: (value: string) => void
  label: string
}

function SearchInput({ value, onChange, label }: SearchInputProps) {
  return (
    <label className="search-field">
      <span className="search-field-label">{label}</span>
      <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5" aria-hidden="true">
        <circle cx="10.5" cy="10.5" r="6.5" />
        <path d="m16 16 4.5 4.5" />
      </svg>
      <input
        type="search"
        placeholder={label}
        value={value}
        onChange={(event) => onChange(event.target.value)}
      />
    </label>
  )
}

export default SearchInput
