import { createContext, useCallback, useEffect, useMemo, useState } from 'react'
import type { ReactNode } from 'react'
import { authApi } from '@features/auth/services/authApi'
import { setBearerToken } from '@shared/utils/api'
import type { LoginCredentials, User } from '@shared/types/api'

interface AuthContextType {
  user: User | null
  isAuthenticated: boolean
  isResolving: boolean
  login: (credentials: LoginCredentials) => Promise<void>
  logout: () => Promise<void>
}

export const AuthContext = createContext<AuthContextType>(null!)

// Auth state is memory-only in this slice, so initial resolution always
// lands on logged-out; the silent-refresh slice will replace this default
// with a refresh call. The seam exists so the routing guard can cover the
// resolving state (full-page spinner) instead of flashing protected
// content or the login form.
const resolveToLoggedOut = () => Promise.resolve<User | null>(null)

export function AuthProvider({
  children,
  resolveInitialState = resolveToLoggedOut,
}: {
  children: ReactNode
  resolveInitialState?: () => Promise<User | null>
}) {
  const [user, setUser] = useState<User | null>(null)
  const [isResolving, setIsResolving] = useState(true)

  useEffect(() => {
    let cancelled = false
    resolveInitialState().then((resolvedUser) => {
      if (cancelled) return
      setUser(resolvedUser)
      setIsResolving(false)
    })
    return () => {
      cancelled = true
    }
  }, [resolveInitialState])

  const login = useCallback(async (credentials: LoginCredentials) => {
    const { token } = await authApi.login(credentials)
    setBearerToken(token)
    setUser({ username: credentials.username })
  }, [])

  // Logout revokes the server-side refresh session, then drops the
  // in-memory token even if the call fails — a failed exit must not
  // strand the user in the app. (The backend clears the cookie before
  // revoking, so a failed call usually means the session was already
  // unreachable, and logout is idempotent on retry.)
  const logout = useCallback(async () => {
    try {
      await authApi.logout()
    } catch {
      // Revocation was best-effort; local state must still be dropped.
    } finally {
      setBearerToken(null)
      setUser(null)
    }
  }, [])

  const value = useMemo(
    () => ({ user, isAuthenticated: user !== null, isResolving, login, logout }),
    [user, isResolving, login, logout],
  )

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>
}
