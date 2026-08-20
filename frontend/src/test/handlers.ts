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
]
