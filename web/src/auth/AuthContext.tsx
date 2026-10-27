import { useQuery, useQueryClient } from '@tanstack/react-query'
import { createContext, useCallback, useContext, useMemo, useState, type ReactNode } from 'react'
import { authApi } from '../api/auth'
import { tokenStore } from '../api/client'
import type { DepartmentDTO, MeDTO } from '../api/types'

interface AuthState {
  token: string | null
  me: MeDTO | undefined
  // The caller's department (name for the header, head_id for request buttons).
  department: DepartmentDTO | undefined
  // A manager is a head only of the department whose head_id is theirs.
  isHead: boolean
  loading: boolean
  error: unknown
  login: (email: string, password: string) => Promise<MeDTO>
  logout: () => void
}

const AuthContext = createContext<AuthState | null>(null)

export function AuthProvider({ children }: { children: ReactNode }) {
  const queryClient = useQueryClient()
  const [token, setToken] = useState(tokenStore.get)

  // A stored token is checked with /auth/me; a 401 there sends the user to
  // the login page (api/client.ts), so expiry needs no timer.
  const meQuery = useQuery({
    queryKey: ['auth', 'me', token],
    queryFn: authApi.me,
    enabled: token !== null,
    staleTime: Infinity,
  })
  const me = meQuery.data

  const departmentId = me?.department_id ?? null
  const departmentQuery = useQuery({
    queryKey: ['departments', departmentId],
    queryFn: () => authApi.department(departmentId!),
    enabled: departmentId !== null,
    staleTime: 5 * 60_000,
  })
  const department = departmentId !== null ? departmentQuery.data : undefined

  const login = useCallback(
    async (email: string, password: string) => {
      const res = await authApi.login(email, password)
      tokenStore.set(res.token)
      queryClient.setQueryData(['auth', 'me', res.token], res.user)
      setToken(res.token)
      return res.user
    },
    [queryClient],
  )

  const logout = useCallback(() => {
    tokenStore.clear()
    setToken(null)
    queryClient.clear()
  }, [queryClient])

  const value = useMemo<AuthState>(
    () => ({
      token,
      me,
      department,
      isHead: me !== undefined && department?.head_id === me.id,
      loading: token !== null && meQuery.isPending,
      error: meQuery.error,
      login,
      logout,
    }),
    [token, me, department, meQuery.isPending, meQuery.error, login, logout],
  )

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>
}

export function useAuth(): AuthState {
  const ctx = useContext(AuthContext)
  if (!ctx) throw new Error('useAuth outside AuthProvider')
  return ctx
}

// useMe is for pages behind RequireAuth, where the user is always loaded.
export function useMe(): MeDTO {
  const { me } = useAuth()
  if (!me) throw new Error('useMe outside RequireAuth')
  return me
}
