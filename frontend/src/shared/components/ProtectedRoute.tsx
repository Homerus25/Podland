import type { ReactNode } from 'react'
import { Navigate, useLocation } from 'react-router-dom'
import { useAuth } from '@features/auth/hooks/useAuth'
import { FullPageSpinner } from './FullPageSpinner'

// Guards every non-login URL. While the initial auth state resolves, only
// a spinner renders — protected content and the login form are both held
// back. An unauthenticated visitor is sent to /login with the requested
// location attached, so deep links survive the login round trip.
export function ProtectedRoute({ children }: { children: ReactNode }) {
  const { isAuthenticated, isResolving } = useAuth()
  const location = useLocation()

  if (isResolving) {
    return <FullPageSpinner />
  }
  if (!isAuthenticated) {
    return <Navigate to="/login" replace state={{ from: location }} />
  }
  return <>{children}</>
}
