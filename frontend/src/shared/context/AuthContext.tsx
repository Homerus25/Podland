import { createContext, useCallback, useMemo, useState } from 'react'
import type { ReactNode } from 'react'
import { authApi } from '@features/auth/services/authApi'
import { setBearerToken } from '@shared/utils/api'
import type { LoginCredentials, User } from '@shared/types/api'

interface AuthContextType {
  user: User | null
  isAuthenticated: boolean
  login: (credentials: LoginCredentials) => Promise<void>
  logout: () => void
}

export const AuthContext = createContext<AuthContextType>(null!)

export function AuthProvider({ children }: { children: ReactNode }) {
  const [user, setUser] = useState<User | null>(null)

  const login = useCallback(async (credentials: LoginCredentials) => {
    const { token } = await authApi.login(credentials)
    setBearerToken(token)
    setUser({ username: credentials.username })
  }, [])

  const logout = useCallback(() => {
    setBearerToken(null)
    setUser(null)
  }, [])

  const value = useMemo(
    () => ({ user, isAuthenticated: user !== null, login, logout }),
    [user, login, logout],
  )

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>
}
