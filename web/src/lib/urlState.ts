import { useEffect } from 'react'
import { useSearchParams } from 'react-router'

export const PAGE_SIZE = 20

type Patch = Record<string, string | number | null | undefined>

// useUrlState keeps a list's tab, filters and page in the address, so going
// back from a card returns to the same place. Changing anything but the page
// starts from the first page.
export function useUrlState() {
  const [params, setParams] = useSearchParams()
  const get = (key: string) => params.get(key) ?? ''
  const set = (patch: Patch) =>
    setParams(
      (prev) => {
        const next = new URLSearchParams(prev)
        if (!('page' in patch)) next.delete('page')
        for (const [k, v] of Object.entries(patch)) {
          if (v === null || v === undefined || v === '') next.delete(k)
          else next.set(k, String(v))
        }
        return next
      },
      { replace: true },
    )

  const page = Number(get('page')) || 1
  const pageSize = Number(get('size')) || PAGE_SIZE
  // Props for an Ant Design Table with server paging.
  const pagination = (total: number | undefined) => ({
    current: page,
    pageSize,
    total: total ?? 0,
    hideOnSinglePage: true,
    showSizeChanger: true,
    onChange: (p: number, size: number) =>
      set({ page: size === pageSize ? p : null, size: size === PAGE_SIZE ? null : size }),
  })

  return { get, set, page, pageSize, offset: (page - 1) * pageSize, pagination }
}

export type UrlState = ReturnType<typeof useUrlState>

// useLastPage moves a list that is past its last page — its last rows were
// deleted, decided or otherwise taken away from under it — back to the last
// one. The table's pager would show the last page number over no rows and
// never ask for it.
export function useLastPage(url: UrlState, total: number | undefined) {
  const { page, pageSize, set } = url
  useEffect(() => {
    if (total && page > 1 && (page - 1) * pageSize >= total) {
      set({ page: Math.ceil(total / pageSize) })
    }
  }, [total, page, pageSize, set])
}
