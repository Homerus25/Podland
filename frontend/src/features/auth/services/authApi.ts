import { api } from '@shared/utils/api'
import type { LoginCredentials, LoginResponse } from '@shared/types/api'

export const authApi = {
  login: async (credentials: LoginCredentials): Promise<LoginResponse> => {
    const response = await api.post<LoginResponse>('/auth/login', credentials)
    return response.data
  },
}
