import { SearchOutlined } from '@ant-design/icons'
import { Input } from 'antd'

// SearchInput applies a search on Enter, on leaving the field and on the
// clear button — not on every key, so the list is not asked per letter.
export function SearchInput({
  value,
  onSearch,
  placeholder,
  width = 320,
}: {
  value: string
  onSearch: (q: string | null) => void
  placeholder: string
  width?: number
}) {
  return (
    <Input
      allowClear
      prefix={<SearchOutlined />}
      placeholder={placeholder}
      defaultValue={value}
      onPressEnter={(e) => onSearch(e.currentTarget.value.trim())}
      // The clear button sends an empty value.
      onChange={(e) => {
        if (e.target.value === '' && value) onSearch(null)
      }}
      onBlur={(e) => {
        if (e.currentTarget.value.trim() !== value) onSearch(e.currentTarget.value.trim())
      }}
      style={{ width }}
    />
  )
}
