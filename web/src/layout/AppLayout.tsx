import { BellOutlined, LogoutOutlined, UserOutlined } from '@ant-design/icons'
import { Badge, Button, Dropdown, Menu, type MenuProps } from 'antd'
import { Link, Outlet, useLocation, useNavigate } from 'react-router'
import { useLatestNotifications } from '../api/notifications'
import { usePendingRequestsCount } from '../api/requests'
import { useAuth, useMe } from '../auth/AuthContext'
import { Logo } from '../components/Logo'
import { UserAvatar } from '../components/UserAvatar'
import { roleLabels } from '../lib/labels'
import { homePath, isGroup, navByRole, type NavLeaf } from '../lib/roles'

// selectedKey picks the menu item for the current page: the longest item path
// that the location starts with (/subscriptions/5 → /subscriptions).
function selectedKey(pathname: string, leaves: NavLeaf[]): string | undefined {
  return leaves
    .map((l) => l.path)
    .filter((p) => pathname === p || pathname.startsWith(p + '/'))
    .sort((a, b) => b.length - a.length)[0]
}

export function AppLayout() {
  const me = useMe()
  const { department, isHead, logout } = useAuth()
  const location = useLocation()
  const navigate = useNavigate()

  // Requests waiting for this user: a department head decides pending_manager,
  // an admin pending_admin; anyone else has nothing to decide.
  const pendingStatus =
    me.role === 'admin' ? 'pending_admin' : me.role === 'manager' && isHead ? 'pending_manager' : null
  const pending = usePendingRequestsCount(pendingStatus)
  const notifications = useLatestNotifications()

  const nav = navByRole[me.role]
  const leaves = nav.flatMap((item) => (isGroup(item) ? item.children : [item]))
  const current = selectedKey(location.pathname, leaves)

  const leafItem = (leaf: NavLeaf) => ({
    key: leaf.path,
    label: (
      <Link to={leaf.path} style={{ display: 'inline-flex', alignItems: 'center', gap: 8 }}>
        {leaf.label}
        {leaf.requestsBadge && <Badge count={pending.data ?? 0} size="small" />}
      </Link>
    ),
  })
  const menuItems: MenuProps['items'] = nav.map((item) =>
    isGroup(item)
      ? { key: item.key, label: item.label, children: item.children.map(leafItem) }
      : leafItem(item),
  )

  const userMenu: MenuProps['items'] = [
    { key: 'profile', icon: <UserOutlined />, label: 'Профиль', onClick: () => navigate('/profile') },
    { type: 'divider' },
    {
      key: 'logout',
      icon: <LogoutOutlined />,
      label: 'Выйти',
      danger: true,
      onClick: () => {
        logout()
        navigate('/login', { replace: true })
      },
    },
  ]

  const subtitle = [roleLabels[me.role], department?.name].filter(Boolean).join(' · ')
  const unread = notifications.data?.unread ?? 0

  return (
    <>
      <header className="app-header">
        <Link to={homePath(me.role)} aria-label="На главную">
          <Logo />
        </Link>
        <Menu
          className="app-menu"
          mode="horizontal"
          items={menuItems}
          selectedKeys={current ? [current] : []}
        />
        <Badge count={unread} size="small" offset={[-6, 6]}>
          <Button
            aria-label={unread ? `Уведомления: ${unread} новых` : 'Уведомления'}
            icon={<BellOutlined style={{ fontSize: 18 }} />}
            style={{ width: 44, background: '#383A40', borderColor: '#383A40' }}
            onClick={() => navigate('/notifications')}
          />
        </Badge>
        <Dropdown menu={{ items: userMenu }} trigger={['click']} placement="bottomRight">
          <div className="app-user" role="button" tabIndex={0} aria-label="Меню профиля">
            <UserAvatar id={me.id} name={me.full_name} />
            <div>
              <div className="app-user-name">{me.full_name}</div>
              <div className="app-user-role">{subtitle}</div>
            </div>
          </div>
        </Dropdown>
      </header>
      <main className="app-main">
        <Outlet />
      </main>
    </>
  )
}
