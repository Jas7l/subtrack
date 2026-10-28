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
import { AuditPage } from './pages/AuditPage'
import { DepartmentsPage } from './pages/DepartmentsPage'
import { LoginPage } from './pages/LoginPage'
import { MyRequestsPage } from './pages/MyRequestsPage'
import { MySubscriptionsPage } from './pages/MySubscriptionsPage'
import { NotFoundPage } from './pages/NotFoundPage'
import { NotificationsPage } from './pages/NotificationsPage'
import { ProfilePage } from './pages/ProfilePage'
import { RecommendationsPage } from './pages/RecommendationsPage'
import { RequestPage } from './pages/RequestPage'
import { RequestsPage } from './pages/RequestsPage'
import { SettingsPage } from './pages/SettingsPage'
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

// Pages of the app and who may open them.
interface PageDef {
  path: string
  roles: readonly Role[]
  Page: ComponentType
}

const pages: PageDef[] = [
  { path: '/my/subscriptions', roles: STAFF, Page: MySubscriptionsPage },
  { path: '/my/requests', roles: STAFF, Page: MyRequestsPage },
  { path: '/requests/:id', roles: ALL, Page: RequestPage },
  { path: '/notifications', roles: ALL, Page: NotificationsPage },
  { path: '/profile', roles: ALL, Page: ProfilePage },
  { path: '/dashboard', roles: BOSSES, Page: DashboardPage },
  { path: '/subscriptions', roles: BOSSES, Page: SubscriptionsPage },
  { path: '/subscriptions/:id', roles: ALL, Page: SubscriptionPage },
  { path: '/requests', roles: BOSSES, Page: RequestsPage },
  { path: '/users', roles: BOSSES, Page: UsersPage },
  { path: '/users/:id', roles: BOSSES, Page: UserPage },
  { path: '/forecast', roles: BOSSES, Page: ForecastPage },
  { path: '/departments', roles: ADMIN, Page: DepartmentsPage },
  { path: '/assignments', roles: ADMIN, Page: AssignmentsPage },
  { path: '/transactions', roles: ADMIN, Page: TransactionsPage },
  { path: '/settings', roles: ADMIN, Page: SettingsPage },
  { path: '/recommendations', roles: ADMIN, Page: RecommendationsPage },
  { path: '/audit', roles: ADMIN, Page: AuditPage },
]

const pageSpin = (
  <div className="page-spin">
    <Spin size="large" />
  </div>
)

// LazyPage gives the pages loaded on demand a spinner meanwhile.
function LazyPage({ Page }: { Page: ComponentType }) {
  return (
    <Suspense fallback={pageSpin}>
      <Page />
    </Suspense>
  )
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
                <LazyPage Page={page.Page} />
              </RequireRole>
            ),
          })),
          { path: '*', element: <NotFoundPage /> },
        ],
      },
    ],
  },
])
