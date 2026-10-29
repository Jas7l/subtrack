import {
  keepPreviousData,
  useMutation,
  useQuery,
  useQueryClient,
  type QueryClient,
} from '@tanstack/react-query'
import { download, request, type Query } from './client'
import type { CreateTransactionBody, Page, PayTransactionBody, TransactionDTO } from './types'

// Payments are the admin's only. Filters: subscription_id, status, from, to,
// order (desc by default; asc — oldest first).
export function useTransactions(query: Query = {}, enabled = true) {
  return useQuery({
    queryKey: ['transactions', 'list', query],
    queryFn: () => request<Page<TransactionDTO>>('/transactions', { query }),
    placeholderData: keepPreviousData,
    enabled,
  })
}

// A payment moves the dashboard's payment numbers and spending by month.
function afterChange(queryClient: QueryClient) {
  void queryClient.invalidateQueries({ queryKey: ['transactions'] })
  void queryClient.invalidateQueries({ queryKey: ['analytics'] })
}

export function useCreateTransaction() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (body: CreateTransactionBody) =>
      request<TransactionDTO>('/transactions', { method: 'POST', body }),
    onSuccess: () => afterChange(queryClient),
  })
}

// Pay and fail refresh even when they fail: a 409 «cannot move to that
// status» means someone else has paid it meanwhile and the row is stale.

// From pending or failed.
export function usePayTransaction() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({ id, body }: { id: number; body: PayTransactionBody }) =>
      request<TransactionDTO>(`/transactions/${id}/pay`, {
        method: 'POST',
        body,
      }),
    onSettled: () => afterChange(queryClient),
  })
}

// Only from pending; a failed payment can still be paid later.
export function useFailTransaction() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (id: number) =>
      request<TransactionDTO>(`/transactions/${id}/fail`, { method: 'POST' }),
    onSettled: () => afterChange(queryClient),
  })
}

// The Excel file takes the same filters as the list (without paging).
export function exportTransactions(query: Query) {
  return download('/transactions/export', query)
}
