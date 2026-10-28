import { Select } from 'antd'
import { useEffect, useState, type CSSProperties } from 'react'
import { useSubscription, useSubscriptions } from '../api/subscriptions'
import type { Query } from '../api/client'
import type { SubscriptionDTO } from '../api/types'
import { seatsText } from '../lib/subscriptions'

const SEARCH_DELAY_MS = 300
const LIMIT = 20

// SubscriptionSelect picks a subscription by searching the server (name or
// vendor), showing the seats taken. `query` narrows the search (e.g. only
// open ones); the chosen one is shown even when the search missed it.
export function SubscriptionSelect({
  value,
  onChange,
  query,
  allowClear = false,
  placeholder = 'Начните вводить название',
  style,
}: {
  value?: number
  onChange?: (id: number | undefined, s: SubscriptionDTO | undefined) => void
  query?: Query
  allowClear?: boolean
  placeholder?: string
  style?: CSSProperties
}) {
  const [search, setSearch] = useState('')
  const [q, setQ] = useState('')
  useEffect(() => {
    const t = setTimeout(() => setQ(search.trim()), SEARCH_DELAY_MS)
    return () => clearTimeout(t)
  }, [search])

  const subs = useSubscriptions({ ...query, q, limit: LIMIT })
  const found = subs.data?.items ?? []
  const missing = value !== undefined && !found.some((s) => s.id === value)
  const chosen = useSubscription(missing ? value : NaN)
  const list = missing && chosen.data ? [chosen.data, ...found] : found

  return (
    <Select<number>
      value={value}
      onChange={(id) => onChange?.(id, list.find((s) => s.id === id))}
      allowClear={allowClear}
      placeholder={placeholder}
      showSearch={{ filterOption: false, onSearch: setSearch }}
      loading={subs.isFetching}
      options={list.map((s) => ({
        value: s.id,
        label: (
          <span>
            {s.name} <span className="cell-sub">· {s.department_name ?? 'общая'} · места {seatsText(s)}</span>
          </span>
        ),
      }))}
      notFoundContent={subs.isFetching ? 'Ищем…' : 'Ничего не нашли'}
      style={{ width: '100%', ...style }}
    />
  )
}
