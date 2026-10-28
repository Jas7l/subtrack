import { Select } from 'antd'
import { useEffect, useState, type CSSProperties } from 'react'
import type { Query } from '../api/client'
import { useUser, useUsers } from '../api/users'
import { useMe } from '../auth/AuthContext'

const SEARCH_DELAY_MS = 300
const LIMIT = 20

// UserSelect picks a user by searching the server (`q`), not by loading
// everybody: the admin's list can be long. The server already limits a
// manager to their department. `exclude` hides those who must not be offered
// (e.g. already holding the licence); `query` narrows the search (role);
// `includeBlocked` is for filters, where blocked people count too. The chosen
// user is shown even when the search did not return them (a filter from the
// address, the current head).
export function UserSelect({
  value,
  onChange,
  exclude,
  query,
  includeBlocked = false,
  allowClear = false,
  placeholder = 'Начните вводить ФИО или email',
  style,
}: {
  value?: number
  onChange?: (id: number | undefined) => void
  exclude?: ReadonlySet<number>
  query?: Query
  includeBlocked?: boolean
  allowClear?: boolean
  placeholder?: string
  style?: CSSProperties
}) {
  const me = useMe()
  const [search, setSearch] = useState('')
  const [q, setQ] = useState('')
  useEffect(() => {
    const t = setTimeout(() => setQ(search.trim()), SEARCH_DELAY_MS)
    return () => clearTimeout(t)
  }, [search])

  const users = useUsers({ ...query, active: includeBlocked ? undefined : true, q, limit: LIMIT })
  const found = (users.data?.items ?? []).filter((u) => !exclude?.has(u.id))
  const missing = value !== undefined && !found.some((u) => u.id === value)
  const chosen = useUser(missing ? value : NaN)
  const list = missing && chosen.data ? [chosen.data, ...found] : found

  const options = list.map((u) => ({
    value: u.id,
    label: (
      <span>
        {u.full_name}
        {u.id === me.id && ' (вы)'} <span className="cell-sub">· {u.email}</span>
      </span>
    ),
  }))

  return (
    <Select<number>
      value={value}
      onChange={(id) => onChange?.(id)}
      allowClear={allowClear}
      placeholder={placeholder}
      showSearch={{ filterOption: false, onSearch: setSearch }}
      loading={users.isFetching}
      options={options}
      notFoundContent={users.isFetching ? 'Ищем…' : 'Никого не нашли'}
      style={{ width: '100%', ...style }}
    />
  )
}
