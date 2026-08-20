export interface User {
  username: string
}

export interface LoginCredentials {
  username: string
  password: string
}

export interface LoginResponse {
  token: string
}

// Backend error envelope (backend/internal/errors/handlers.go ErrorResponse)
export interface ApiErrorEnvelope {
  requestId: string
  error: {
    message: string
    code: string
  }
}

// The display-ready form of an error: code + human-readable message.
export interface ApiError {
  code: string
  message: string
}
