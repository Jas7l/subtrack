import { Select } from 'antd'
import type { CSSProperties } from 'react'
import { useDepartments } from '../api/departments'

// DepartmentSelect picks one of the departments (the admin sees them all).
// Cleared, it gives undefined: «no department» / company-wide, as the
// placeholder says.
export function DepartmentSelect({
  value,
  onChange,
  placeholder = 'Без отдела',
  style,
}: {
  value?: number
  onChange?: (id: number | undefined) => void
  placeholder?: string
  style?: CSSProperties
}) {
  const departments = useDepartments()
  return (
    <Select<number>
      allowClear
      value={value}
      onChange={(id) => onChange?.(id)}
      placeholder={placeholder}
      loading={departments.isPending}
      showSearch={{ optionFilterProp: 'label' }}
      options={(departments.data?.items ?? []).map((d) => ({ value: d.id, label: d.name }))}
      style={{ width: '100%', ...style }}
    />
  )
}
