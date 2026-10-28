import { CheckOutlined } from '@ant-design/icons'
import { Alert, App, Button, Card, Empty, Pagination, Segmented, Spin } from 'antd'
import { useNavigate } from 'react-router'
import { useMarkAllRead, useNotifications } from '../api/notifications'
import type { NotificationDTO } from '../api/types'
import { NotificationItem } from '../components/NotificationItem'
import { PageHeader } from '../components/PageHeader'
import { errorText } from '../lib/errors'
import { useOpenNotification } from '../lib/notifications'
import { useLastPage, useUrlState } from '../lib/urlState'

// NotificationsPage: all of the user's notifications, newest first. The
// filter and the page are in the address, as on the other lists: back from
// what a notification opened returns to the same place.
export function NotificationsPage() {
  const navigate = useNavigate()
  const { message } = App.useApp()
  const url = useUrlState()
  const unreadOnly = url.get('unread') === '1'

  const list = useNotifications({
    unread: unreadOnly || undefined,
    limit: url.pageSize,
    offset: url.offset,
  })
  // Reading the unread ones of the last page empties it.
  useLastPage(url, list.data?.total)
  const openNotification = useOpenNotification()
  const markAll = useMarkAllRead()

  const unread = list.data?.unread ?? 0
  const items = list.data?.items ?? []

  // A click marks the notification read and opens what it is about; old
  // notifications without a link just get marked.
  const open = (n: NotificationDTO) => {
    const link = openNotification(n)
    if (link) navigate(link)
  }

  return (
    <>
      <PageHeader
        title="Уведомления"
        subtitle={unread > 0 ? `Непрочитанных: ${unread}` : 'Всё прочитано'}
        extra={
          <>
            <Segmented
              value={unreadOnly ? 'unread' : 'all'}
              options={[
                { value: 'all', label: 'Все' },
                { value: 'unread', label: 'Непрочитанные' },
              ]}
              onChange={(v) => url.set({ unread: v === 'unread' ? 1 : null })}
            />
            <Button
              icon={<CheckOutlined />}
              disabled={unread === 0}
              loading={markAll.isPending}
              onClick={() =>
                markAll.mutate(undefined, { onError: (err) => message.error(errorText(err)) })
              }
            >
              Прочитать все
            </Button>
          </>
        }
      />
      {list.error && (
        <Alert type="error" showIcon title={errorText(list.error)} style={{ marginBottom: 24 }} />
      )}
      <Card variant="borderless" styles={{ body: { padding: 8 } }}>
        {list.isPending ? (
          <div className="page-spin">
            <Spin size="large" />
          </div>
        ) : items.length === 0 ? (
          <Empty
            style={{ padding: 32 }}
            description={unreadOnly ? 'Непрочитанных уведомлений нет' : 'Уведомлений пока нет'}
          />
        ) : (
          items.map((n) => <NotificationItem key={n.id} item={n} onClick={open} />)
        )}
      </Card>
      <Pagination
        style={{ marginTop: 20, justifyContent: 'center' }}
        current={url.page}
        pageSize={url.pageSize}
        total={list.data?.total ?? 0}
        hideOnSinglePage
        showSizeChanger={false}
        onChange={(page) => url.set({ page })}
      />
    </>
  )
}
