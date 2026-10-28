import { Select } from 'antd'
import { useEffect, useState } from 'react'
import { useUsers } from '../api/users'
import { useMe } from '../auth/AuthContext'

const SEARCH_DELAY_MS = 300
const LIMIT = 20

// UserSelect picks an active user by searching the server (`q`), not by
// loading everybody: the admin's list (К4) can be long. The server already
// limits a manager to their department. `exclude` hides those who must not
// be offered (e.g. already holding the licence).
export function UserSelect({
  value,
  onChange,
  exclude,
  placeholder = 'Начните вводить ФИО или email',
}: {
  value?: number
  onChange?: (id: number) => void
  exclude?: ReadonlySet<number>
  placeholder?: string
}) {
  const me = useMe()
  const [search, setSearch] = useState('')
  const [q, setQ] = useState('')
  useEffect(() => {
    const t = setTimeout(() => setQ(search.trim()), SEARCH_DELAY_MS)
    return () => clearTimeout(t)
  }, [search])

  const users = useUsers({ active: true, q, limit: LIMIT })
  const options = (users.data?.items ?? [])
    .filter((u) => !exclude?.has(u.id))
    .map((u) => ({
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
      onChange={onChange}
      placeholder={placeholder}
      showSearch={{ filterOption: false, onSearch: setSearch }}
      loading={users.isFetching}
      options={options}
      notFoundContent={users.isFetching ? 'Ищем…' : 'Никого не нашли'}
      style={{ width: '100%' }}
    />
  )
}
