import { http, HttpResponse } from 'msw'

// Handlers mirror the real API shapes from the backend
// (see backend/internal/app/app.go). /api/health responds 200 "OK".
export const handlers = [
  http.get('*/api/health', () => {
    return new HttpResponse('OK', { status: 200 })
  }),
]
