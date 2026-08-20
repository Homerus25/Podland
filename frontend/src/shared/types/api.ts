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

export type MachinePermission = 'read' | 'write' | 'admin'

// Machine as returned by GET /api/machines and /api/machines/{id}
// (backend/internal/domain/machines/service.go MachineResponse).
export interface Machine {
  id: string
  name: string
  description: string
  hostname: string
  port: number
  sshUsername: string
  sshKeyFingerprint: string
  podmanSocketPath: string
  ownerId: string
  permission: MachinePermission
  createdAt: string
  updatedAt: string
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
