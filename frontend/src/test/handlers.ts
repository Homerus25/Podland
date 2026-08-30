import { http, HttpResponse } from 'msw'

// Handlers mirror the real API shapes from the backend
// (see backend/internal/app/app.go and backend/internal/domain/auth/handler.go).
export const loginSuccessHandler = http.post('*/api/auth/login', () => {
  return HttpResponse.json({ token: 'test-access-token' })
})

export const handlers = [
  http.get('*/api/health', () => {
    return new HttpResponse('OK', { status: 200 })
  }),
  loginSuccessHandler,
  http.post('*/api/auth/logout', () => {
    return HttpResponse.json({ message: 'Logged out successfully' })
  }),
  // Default status probe so card-rendering tests see a healthy machine;
  // individual tests override this with server.use().
  http.get('*/api/machines/:id/status', () => {
    return HttpResponse.json({ status: 'online', checkedAt: '2026-08-29T10:00:00Z' })
  }),
]
