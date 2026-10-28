import { keepPreviousData, useQuery } from '@tanstack/react-query'
import { request, type Query } from './client'
import type { AuditEntryDTO, Page } from './types'

// The audit log is the admin's only, newest first. Filters: actor_id,
// entity_type, entity_id, action, from, to.
export function useAudit(query: Query = {}, enabled = true) {
  return useQuery({
    queryKey: ['audit', query],
    queryFn: () => request<Page<AuditEntryDTO>>('/audit', { query }),
    placeholderData: keepPreviousData,
    enabled,
  })
}
