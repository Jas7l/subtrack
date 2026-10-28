import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { request } from './client'
import type { NotificationSettingsBody, NotificationSettingsDTO } from './types'

// How many days before the end date and the next payment the scheduler warns.
export function useNotificationSettings() {
  return useQuery({
    queryKey: ['settings', 'notifications'],
    queryFn: () => request<NotificationSettingsDTO>('/settings/notifications'),
  })
}

export function useUpdateNotificationSettings() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (body: NotificationSettingsBody) =>
      request<NotificationSettingsDTO>('/settings/notifications', {
        method: 'PUT',
        body,
      }),
    onSuccess: (data) => queryClient.setQueryData(['settings', 'notifications'], data),
  })
}
