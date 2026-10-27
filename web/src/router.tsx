import type { ComponentType } from 'react'
import { Navigate, Outlet, createBrowserRouter } from 'react-router'
import type { Role } from './api/types'
import { AuthProvider, useMe } from './auth/AuthContext'
import { RequireAuth } from './auth/RequireAuth'
import { RequireRole } from './auth/RequireRole'
import { AppLayout } from './layout/AppLayout'
import { homePath } from './lib/roles'
import { LoginPage } from './pages/LoginPage'
import { MyRequestsPage } from './pages/MyRequestsPage'
import { MySubscriptionsPage } from './pages/MySubscriptionsPage'
import { NotFoundPage } from './pages/NotFoundPage'
import { NotificationsPage } from './pages/NotificationsPage'
import { ProfilePage } from './pages/ProfilePage'
import { RequestPage } from './pages/RequestPage'
import { StubPage } from './pages/StubPage'

const ALL: readonly Role[] = ['admin', 'manager', 'employee']
const STAFF: readonly Role[] = ['manager', 'employee']
const BOSSES: readonly Role[] = ['admin', 'manager']
const ADMIN: readonly Role[] = ['admin']

// Pages of the app and who may open them. Until a page is built (no `Page`)
// it is a stub that names the stage it belongs to (CLIENT_PLAN.md, section 7).
interface PageDef {
  path: string
  roles: readonly Role[]
  title: string | Partial<Record<Role, string>>
  stage: string
  Page?: ComponentType
}

const pages: PageDef[] = [
  {
    path: '/my/subscriptions',
    roles: STAFF,
    title: 'Мои подписки',
    stage: 'К2',
    Page: MySubscriptionsPage,
  },
  { path: '/my/requests', roles: STAFF, title: 'Мои заявки', stage: 'К2', Page: MyRequestsPage },
  { path: '/requests/:id', roles: ALL, title: 'Заявка', stage: 'К2', Page: RequestPage },
  {
    path: '/notifications',
    roles: ALL,
    title: 'Уведомления',
    stage: 'К2',
    Page: NotificationsPage,
  },
  { path: '/profile', roles: ALL, title: 'Профиль', stage: 'К2', Page: ProfilePage },
  {
    path: '/dashboard',
    roles: BOSSES,
    title: { admin: 'Дашборд', manager: 'Бюджет отдела' },
    stage: 'К3',
  },
  {
    path: '/subscriptions',
    roles: BOSSES,
    title: { admin: 'Подписки', manager: 'Подписки отдела' },
    stage: 'К3',
  },
  { path: '/subscriptions/:id', roles: ALL, title: 'Подписка', stage: 'К3' },
  {
    path: '/requests',
    roles: BOSSES,
    title: { admin: 'Все заявки', manager: 'Заявки отдела' },
    stage: 'К3',
  },
  {
    path: '/users',
    roles: BOSSES,
    title: { admin: 'Пользователи', manager: 'Сотрудники отдела' },
    stage: 'К3',
  },
  { path: '/users/:id', roles: BOSSES, title: 'Пользователь', stage: 'К3' },
  { path: '/forecast', roles: BOSSES, title: 'Прогноз бюджета', stage: 'К3' },
  { path: '/departments', roles: ADMIN, title: 'Отделы', stage: 'К4' },
  { path: '/assignments', roles: ADMIN, title: 'Лицензии', stage: 'К4' },
  { path: '/transactions', roles: ADMIN, title: 'Платежи', stage: 'К5' },
  { path: '/settings', roles: ADMIN, title: 'Настройки', stage: 'К5' },
  { path: '/recommendations', roles: ADMIN, title: 'Рекомендации', stage: 'К6' },
  { path: '/audit', roles: ADMIN, title: 'Аудит', stage: 'К7' },
]

function Stub({ page }: { page: PageDef }) {
  const me = useMe()
  const title = typeof page.title === 'string' ? page.title : (page.title[me.role] ?? '')
  return <StubPage title={title} stage={page.stage} />
}

function HomeRedirect() {
  const me = useMe()
  return <Navigate to={homePath(me.role)} replace />
}

function Root() {
  return (
    <AuthProvider>
      <Outlet />
    </AuthProvider>
  )
}

export const router = createBrowserRouter([
  {
    element: <Root />,
    children: [
      { path: '/login', element: <LoginPage /> },
      {
        element: (
          <RequireAuth>
            <AppLayout />
          </RequireAuth>
        ),
        children: [
          { index: true, element: <HomeRedirect /> },
          ...pages.map((page) => ({
            path: page.path,
            element: (
              <RequireRole roles={page.roles}>
                {page.Page ? <page.Page /> : <Stub page={page} />}
              </RequireRole>
            ),
          })),
          { path: '*', element: <NotFoundPage /> },
        ],
      },
    ],
  },
])
