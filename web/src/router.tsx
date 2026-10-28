import { Spin } from 'antd'
import { lazy, Suspense, type ComponentType } from 'react'
import { Navigate, Outlet, createBrowserRouter } from 'react-router'
import type { Role } from './api/types'
import { AuthProvider, useMe } from './auth/AuthContext'
import { RequireAuth } from './auth/RequireAuth'
import { RequireRole } from './auth/RequireRole'
import { AppLayout } from './layout/AppLayout'
import { homePath } from './lib/roles'
import { AssignmentsPage } from './pages/AssignmentsPage'
import { DepartmentsPage } from './pages/DepartmentsPage'
import { LoginPage } from './pages/LoginPage'
import { MyRequestsPage } from './pages/MyRequestsPage'
import { MySubscriptionsPage } from './pages/MySubscriptionsPage'
import { NotFoundPage } from './pages/NotFoundPage'
import { NotificationsPage } from './pages/NotificationsPage'
import { ProfilePage } from './pages/ProfilePage'
import { RequestPage } from './pages/RequestPage'
import { RequestsPage } from './pages/RequestsPage'
import { SettingsPage } from './pages/SettingsPage'
import { StubPage } from './pages/StubPage'
import { SubscriptionPage } from './pages/SubscriptionPage'
import { SubscriptionsPage } from './pages/SubscriptionsPage'
import { TransactionsPage } from './pages/TransactionsPage'
import { UserPage } from './pages/UserPage'
import { UsersPage } from './pages/UsersPage'

// Pages with charts are loaded on demand: recharts stays out of the main
// bundle.
const DashboardPage = lazy(() =>
  import('./pages/DashboardPage').then((m) => ({ default: m.DashboardPage })),
)
const ForecastPage = lazy(() =>
  import('./pages/ForecastPage').then((m) => ({ default: m.ForecastPage })),
)

const ALL: readonly Role[] = ['admin', 'manager', 'employee']
const STAFF: readonly Role[] = ['manager', 'employee']
const BOSSES: readonly Role[] = ['admin', 'manager']
const ADMIN: readonly Role[] = ['admin']

// Pages of the app and who may open them. Until a page is built (no `Page`)
// it is a stub that names the stage it belongs to (CLIENT_PLAN.md, section 7).
// `builtFor` limits a built page to some roles: the others still see the
// stub (the admin's versions come later).
interface PageDef {
  path: string
  roles: readonly Role[]
  title: string | Partial<Record<Role, string>>
  stage: string
  Page?: ComponentType
  builtFor?: readonly Role[]
}

const MANAGER: readonly Role[] = ['manager']

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
    stage: 'К6',
    Page: DashboardPage,
    builtFor: MANAGER,
  },
  {
    path: '/subscriptions',
    roles: BOSSES,
    title: { admin: 'Подписки', manager: 'Подписки отдела' },
    stage: 'К4',
    Page: SubscriptionsPage,
  },
  {
    path: '/subscriptions/:id',
    roles: ALL,
    title: 'Подписка',
    stage: 'К4',
    Page: SubscriptionPage,
  },
  {
    path: '/requests',
    roles: BOSSES,
    title: { admin: 'Все заявки', manager: 'Заявки отдела' },
    stage: 'К4',
    Page: RequestsPage,
  },
  {
    path: '/users',
    roles: BOSSES,
    title: { admin: 'Пользователи', manager: 'Сотрудники отдела' },
    stage: 'К4',
    Page: UsersPage,
  },
  {
    path: '/users/:id',
    roles: BOSSES,
    title: 'Пользователь',
    stage: 'К4',
    Page: UserPage,
  },
  {
    path: '/forecast',
    roles: BOSSES,
    title: 'Прогноз бюджета',
    stage: 'К6',
    Page: ForecastPage,
    builtFor: MANAGER,
  },
  { path: '/departments', roles: ADMIN, title: 'Отделы', stage: 'К4', Page: DepartmentsPage },
  { path: '/assignments', roles: ADMIN, title: 'Лицензии', stage: 'К4', Page: AssignmentsPage },
  {
    path: '/transactions',
    roles: ADMIN,
    title: 'Платежи',
    stage: 'К5',
    Page: TransactionsPage,
  },
  { path: '/settings', roles: ADMIN, title: 'Настройки', stage: 'К5', Page: SettingsPage },
  { path: '/recommendations', roles: ADMIN, title: 'Рекомендации', stage: 'К6' },
  { path: '/audit', roles: ADMIN, title: 'Аудит', stage: 'К7' },
]

const pageSpin = (
  <div className="page-spin">
    <Spin size="large" />
  </div>
)

// RolePage renders the page built for the user's role, or a stub.
function RolePage({ page }: { page: PageDef }) {
  const me = useMe()
  const { Page } = page
  if (Page && (!page.builtFor || page.builtFor.includes(me.role))) {
    return (
      <Suspense fallback={pageSpin}>
        <Page />
      </Suspense>
    )
  }
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
                <RolePage page={page} />
              </RequireRole>
            ),
          })),
          { path: '*', element: <NotFoundPage /> },
        ],
      },
    ],
  },
])
