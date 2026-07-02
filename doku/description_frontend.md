# Podman Management Frontend: Implementation Guide

## Overview

This document provides the complete frontend implementation specification for the Podman Management Web-App. It describes the React application architecture, components, state management, and integration with the backend API.

---

## Project Structure

```
frontend/
├── public/
│   ├── index.html
│   ├── favicon.ico
│   └── assets/
│       └── logo.svg
├── src/
│   ├── features/
│   │   ├── auth/
│   │   │   ├── components/
│   │   │   │   ├── LoginForm.tsx
│   │   │   │   ├── RegisterForm.tsx
│   │   │   │   └── AuthLayout.tsx
│   │   │   ├── hooks/
│   │   │   │   ├── useAuth.ts
│   │   │   │   ├── useLogin.ts
│   │   │   │   └── useRegister.ts
│   │   │   ├── services/
│   │   │   │   └── authApi.ts
│   │   │   ├── types.ts
│   │   │   └── index.ts
│   │   ├── machines/
│   │   │   ├── components/
│   │   │   │   ├── MachineList.tsx
│   │   │   │   ├── MachineCard.tsx
│   │   │   │   ├── MachineForm.tsx
│   │   │   │   ├── MachineDetails.tsx
│   │   │   │   └── ShareMachineModal.tsx
│   │   │   ├── hooks/
│   │   │   │   ├── useMachines.ts
│   │   │   │   ├── useMachine.ts
│   │   │   │   ├── useCreateMachine.ts
│   │   │   │   ├── useUpdateMachine.ts
│   │   │   │   └── useDeleteMachine.ts
│   │   │   ├── services/
│   │   │   │   └── machinesApi.ts
│   │   │   ├── types.ts
│   │   │   └── index.ts
│   │   ├── containers/
│   │   │   ├── components/
│   │   │   │   ├── ContainerList.tsx
│   │   │   │   ├── ContainerCard.tsx
│   │   │   │   ├── ContainerForm.tsx
│   │   │   │   ├── ContainerActions.tsx
│   │   │   │   └── ContainerLogs.tsx
│   │   │   ├── hooks/
│   │   │   │   ├── useContainers.ts
│   │   │   │   ├── useContainer.ts
│   │   │   │   ├── useStartContainer.ts
│   │   │   │   ├── useStopContainer.ts
│   │   │   │   ├── useCreateContainer.ts
│   │   │   │   └── useDeleteContainer.ts
│   │   │   ├── services/
│   │   │   │   └── containersApi.ts
│   │   │   ├── types.ts
│   │   │   └── index.ts
│   │   ├── images/
│   │   │   ├── components/
│   │   │   │   ├── ImageList.tsx
│   │   │   │   ├── ImageCard.tsx
│   │   │   │   ├── ImagePullForm.tsx
│   │   │   │   └── ImageProgress.tsx
│   │   │   ├── hooks/
│   │   │   │   ├── useImages.ts
│   │   │   │   ├── usePullImage.ts
│   │   │   │   └── useDeleteImage.ts
│   │   │   ├── services/
│   │   │   │   └── imagesApi.ts
│   │   │   ├── types.ts
│   │   │   └── index.ts
│   │   ├── quadlets/
│   │   │   ├── components/
│   │   │   │   ├── QuadletList.tsx
│   │   │   │   ├── QuadletCard.tsx
│   │   │   │   ├── QuadletForm.tsx
│   │   │   │   ├── QuadletActions.tsx
│   │   │   │   ├── QuadletLogs.tsx
│   │   │   │   └── QuadletGenerator.tsx
│   │   │   ├── hooks/
│   │   │   │   ├── useQuadlets.ts
│   │   │   │   ├── useQuadlet.ts
│   │   │   │   ├── useCreateQuadlet.ts
│   │   │   │   ├── useUpdateQuadlet.ts
│   │   │   │   ├── useDeleteQuadlet.ts
│   │   │   │   ├── useStartQuadlet.ts
│   │   │   │   ├── useStopQuadlet.ts
│   │   │   │   └── useGenerateQuadlet.ts
│   │   │   ├── services/
│   │   │   │   └── quadletsApi.ts
│   │   │   ├── types.ts
│   │   │   └── index.ts
│   │   └── events/
│   │       ├── hooks/
│   │       │   └── useEvents.ts
│   │       ├── services/
│   │       │   └── eventsApi.ts
│   │       └── eventBus.ts
│   ├── shared/
│   │   ├── components/
│   │   │   ├── Layout/
│   │   │   │   ├── Sidebar.tsx
│   │   │   │   ├── Header.tsx
│   │   │   │   └── index.tsx
│   │   │   ├── ui/
│   │   │   │   ├── Button.tsx
│   │   │   │   ├── Input.tsx
│   │   │   │   ├── Modal.tsx
│   │   │   │   ├── Card.tsx
│   │   │   │   ├── Badge.tsx
│   │   │   │   ├── Alert.tsx
│   │   │   │   ├── Spinner.tsx
│   │   │   │   └── ProgressBar.tsx
│   │   │   ├── ErrorDisplay.tsx
│   │   │   ├── LoadingSpinner.tsx
│   │   │   ├── ConfirmationModal.tsx
│   │   │   ├── ProtectedRoute.tsx
│   │   │   └── OfflineBanner.tsx
│   │   ├── hooks/
│   │   │   ├── useError.ts
│   │   │   ├── useLocalStorage.ts
│   │   │   └── useDebounce.ts
│   │   ├── context/
│   │   │   ├── ErrorContext.tsx
│   │   │   ├── ThemeContext.tsx
│   │   │   └── AuthContext.tsx
│   │   ├── utils/
│   │   │   ├── api.ts
│   │   │   ├── constants.ts
│   │   │   ├── formatters.ts
│   │   │   └── validators.ts
│   │   └── types/
│   │       ├── api.ts
│   │       ├── machine.ts
│   │       ├── container.ts
│   │       ├── image.ts
│   │       └── index.ts
│   ├── pages/
│   │   ├── DashboardPage.tsx
│   │   ├── LoginPage.tsx
│   │   ├── RegisterPage.tsx
│   │   ├── MachinesPage.tsx
│   │   ├── MachineDetailPage.tsx
│   │   ├── ContainersPage.tsx
│   │   ├── ImagesPage.tsx
│   │   ├── QuadletsPage.tsx
│   │   ├── SettingsPage.tsx
│   │   ├── OfflinePage.tsx
│   │   └── NotFoundPage.tsx
│   ├── App.tsx
│   ├── main.tsx
│   └── theme.ts
├── .env
├── .env.example
├── .eslintrc.json
├── .prettierrc
├── package.json
├── tsconfig.json
├── tsconfig.node.json
├── vite.config.ts
└── index.html
```

---

## Technical Stack

| Category | Technology | Purpose |
|----------|------------|---------|
| Framework | React 18+ | UI framework |
| State Management | React Query | Server state, caching, mutations |
| Styling | Chakra UI | Component library, theming, dark mode |
| Routing | React Router v6 | Client-side routing |
| Build Tool | Vite | Fast builds, HMR, ESM |
| Language | TypeScript | Type safety |
| Real-time | EventSource API | SSE client |
| Streaming | Fetch API + AbortController | Progress streaming |

---

## Configuration

### Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `VITE_API_BASE_URL` | `/api` | Backend API base URL |
| `VITE_APP_NAME` | `Podland` | Application name |
| `VITE_DEFAULT_THEME` | `system` | Default theme: light, dark, or system |

### Vite Configuration

```typescript
// vite.config.ts
import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import path from 'path'

export default defineConfig({
  plugins: [react()],
  resolve: {
    alias: {
      '@': path.resolve(__dirname, './src'),
      '@features': path.resolve(__dirname, './src/features'),
      '@shared': path.resolve(__dirname, './src/shared'),
    },
  },
  server: {
    port: 3000,
    proxy: {
      '/api': {
        target: 'http://localhost:8080',
        changeOrigin: true,
      },
    },
  },
  build: {
    outDir: 'dist',
    sourcemap: true,
  },
})
```

### TypeScript Configuration

```json
{
  "compilerOptions": {
    "target": "ES2020",
    "useDefineForClassFields": true,
    "lib": ["ES2020", "DOM", "DOM.Iterable"],
    "module": "ESNext",
    "skipLibCheck": true,
    "moduleResolution": "bundler",
    "allowImportingTsExtensions": true,
    "resolveJsonModule": true,
    "isolatedModules": true,
    "noEmit": true,
    "jsx": "react-jsx",
    "strict": true,
    "noUnusedLocals": true,
    "noUnusedParameters": true,
    "noFallthroughCasesInSwitch": true,
    "baseUrl": ".",
    "paths": {
      "@/*": ["src/*"],
      "@features/*": ["src/features/*"],
      "@shared/*": ["src/shared/*"]
    }
  },
  "include": ["src"],
  "exclude": ["node_modules"]
}
```

---

## Chakra UI Theme Configuration

```typescript
// src/theme.ts
import { extendTheme, type ThemeConfig } from '@chakra-ui/react'

const config: ThemeConfig = {
  initialColorMode: 'system',
  useSystemColorMode: true,
}

const colors = {
  brand: {
    50: '#e0f2fe',
    100: '#bae6fd',
    500: '#0ea5e9',
    600: '#0284c7',
    700: '#0369a1',
    800: '#075985',
  },
}

const styles = {
  global: {
    body: {
      bg: 'gray.50',
      _dark: {
        bg: 'gray.800',
      },
    },
  },
}

const components = {
  Button: {
    baseStyle: {
      _focus: {
        boxShadow: 'none',
      },
    },
  },
}

export const theme = extendTheme({
  config,
  colors,
  styles,
  components,
})
```

---

## Styling Approach

### CSS Variables
Chakra UI automatically provides CSS variables for theming. Custom variables can be added:

```typescript
// In theme.ts
const theme = extendTheme({
  config,
  colors: {
    ...colors,
    custom: {
      primary: '#3182CE',
      secondary: '#38B2AC',
    },
  },
})
```

### Dark Mode
- **Strategy**: System preference by default, user can override
- **Storage**: User preference stored in localStorage
- **Implementation**: Chakra UI's built-in `useColorMode` hook

```typescript
// Component usage
import { useColorMode, Button } from '@chakra-ui/react'

function ThemeToggle() {
  const { colorMode, toggleColorMode } = useColorMode()
  return (
    <Button onClick={toggleColorMode}>
      {colorMode === 'light' ? 'Dark' : 'Light'}
    </Button>
  )
}
```

---

## State Management with React Query

### Query Client Setup

```typescript
// src/main.tsx
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { ReactQueryDevtools } from '@tanstack/react-query-devtools'

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      refetchOnWindowFocus: false,
      retry: 1,
      staleTime: 5 * 60 * 1000, // 5 minutes
    },
    mutations: {
      retry: 0, // Fail fast
    },
  },
})

ReactDOM.createRoot(document.getElementById('root')!).render(
  <QueryClientProvider client={queryClient}>
    <ChakraProvider theme={theme}>
      <App />
    </ChakraProvider>
    <ReactQueryDevtools initialIsOpen={false} />
  </QueryClientProvider>
)
```

### Custom Hooks Pattern

```typescript
// Example: features/machines/hooks/useMachines.ts
import { useQuery } from '@tanstack/react-query'
import { machinesApi } from '../services/machinesApi'

export function useMachines() {
  return useQuery({
    queryKey: ['machines'],
    queryFn: machinesApi.listMachines,
    // Optionally add staleTime, cacheTime, etc.
  })
}

export function useMachine(machineId: string) {
  return useQuery({
    queryKey: ['machine', machineId],
    queryFn: () => machinesApi.getMachine(machineId),
    enabled: !!machineId,
  })
}
```

### Mutation Hooks

```typescript
// Example: features/machines/hooks/useCreateMachine.ts
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { machinesApi } from '../services/machinesApi'
import { useError } from '@shared/hooks/useError'

export function useCreateMachine() {
  const queryClient = useQueryClient()
  const { showError } = useError()
  
  return useMutation({
    mutationFn: machinesApi.createMachine,
    onSuccess: () => {
      // Invalidate machines list to refetch
      queryClient.invalidateQueries({ queryKey: ['machines'] })
    },
    onError: (error) => {
      showError(error)
    },
  })
}
```

---

## Authentication

### Auth Types

```typescript
// src/shared/types/api.ts
export interface User {
  id: string
  username: string
  createdAt: string
}

export interface AuthState {
  user: User | null
  isAuthenticated: boolean
  isLoading: boolean
}

export interface LoginCredentials {
  username: string
  password: string
}

export interface LoginResponse {
  user: User
}
```

### Auth Context

```typescript
// src/shared/context/AuthContext.tsx
import { createContext, useContext, useEffect, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { authApi } from '@features/auth/services/authApi'

interface AuthContextType {
  user: User | null
  isAuthenticated: boolean
  isLoading: boolean
  login: (credentials: LoginCredentials) => Promise<void>
  logout: () => Promise<void>
  checkAuth: () => Promise<void>
}

const AuthContext = createContext<AuthContextType>(null!)

export function AuthProvider({ children }: { children: React.ReactNode }) {
  const [user, setUser] = useState<User | null>(null)
  const [isLoading, setIsLoading] = useState(true)
  const navigate = useNavigate()

  const checkAuth = async () => {
    try {
      setIsLoading(true)
      const user = await authApi.checkStatus()
      setUser(user)
    } catch (error) {
      setUser(null)
    } finally {
      setIsLoading(false)
    }
  }

  const login = async (credentials: LoginCredentials) => {
    const user = await authApi.login(credentials)
    setUser(user)
    navigate('/')
  }

  const logout = async () => {
    await authApi.logout()
    setUser(null)
    navigate('/login')
  }

  useEffect(() => {
    checkAuth()
  }, [])

  const value = {
    user,
    isAuthenticated: !!user,
    isLoading,
    login,
    logout,
    checkAuth,
  }

  return (
    <AuthContext.Provider value={value}>
      {children}
    </AuthContext.Provider>
  )
}

export function useAuth() {
  return useContext(AuthContext)
}
```

### Custom useAuth Hook

```typescript
// src/features/auth/hooks/useAuth.ts
export function useAuth() {
  return useContext(AuthContext)
}
```

### Auth Service (API)

```typescript
// src/features/auth/services/authApi.ts
import { api } from '@shared/utils/api'

export const authApi = {
  login: async (credentials: LoginCredentials): Promise<User> => {
    const response = await api.post('/auth/login', credentials)
    return response.data.user
  },

  logout: async (): Promise<void> => {
    await api.post('/auth/logout')
  },

  checkStatus: async (): Promise<User> => {
    const response = await api.get('/auth/status')
    return response.data.user
  },
}
```

### 401 Interceptor

```typescript
// src/shared/utils/api.ts
import axios, { AxiosError, AxiosInstance } from 'axios'

const api: AxiosInstance = axios.create({
  baseURL: import.meta.env.VITE_API_BASE_URL,
  withCredentials: true, // Required for HTTP-only cookies
})

// 401 Interceptor
api.interceptors.response.use(
  (response) => response,
  (error: AxiosError) => {
    if (error.response?.status === 401) {
      // Clear auth state
      localStorage.removeItem('auth')
      // Redirect to login
      window.location.href = '/login'
    }
    return Promise.reject(error)
  }
)

export { api }

export interface ApiError {
  requestId: string
  error: {
    message: string
    code: string
  }
}

// Helper to extract error details
export function getApiError(error: unknown): { message: string; code: string } {
  if (axios.isAxiosError(error)) {
    const apiError = error.response?.data as ApiError
    if (apiError?.error) {
      return {
        message: apiError.error.message,
        code: apiError.error.code,
      }
    }
  }
  return {
    message: 'An unexpected error occurred',
    code: 'INTERNAL_ERROR',
  }
}
```

---

## Error Handling

### Error Context

```typescript
// src/shared/context/ErrorContext.tsx
import { createContext, useCallback, useContext, useState } from 'react'

interface ErrorContextType {
  error: { message: string; code: string } | null
  showError: (error: { message: string; code: string }) => void
  clearError: () => void
}

const ErrorContext = createContext<ErrorContextType>(null!)

export function ErrorProvider({ children }: { children: React.ReactNode }) {
  const [error, setError] = useState<{ message: string; code: string } | null>(null)

  const showError = useCallback((error: { message: string; code: string }) => {
    setError(error)
  }, [])

  const clearError = useCallback(() => {
    setError(null)
  }, [])

  return (
    <ErrorContext.Provider value={{ error, showError, clearError }}>
      {children}
      {error && (
        <ErrorDisplay
          message={error.message}
          code={error.code}
          onClose={clearError}
        />
      )}
    </ErrorContext.Provider>
  )
}

export function useError() {
  return useContext(ErrorContext)
}
```

### Error Display Component

```typescript
// src/shared/components/ErrorDisplay.tsx
import { Alert, AlertIcon, AlertTitle, AlertDescription, CloseButton } from '@chakra-ui/react'

interface ErrorDisplayProps {
  message: string
  code: string
  onClose: () => void
}

export function ErrorDisplay({ message, code, onClose }: ErrorDisplayProps) {
  // For 5xx errors, show generic message
  const isServerError = code.startsWith('INTERNAL_') || code.startsWith('SERVICE_')
  const displayMessage = isServerError ? 'An unexpected error occurred' : message

  return (
    <Alert status="error" variant="subtle" mt={4}>
      <AlertIcon />
      <Box flex="1">
        <AlertTitle>{code}</AlertTitle>
        <AlertDescription display="block">{displayMessage}</AlertDescription>
      </Box>
      <CloseButton onClick={onClose} position="absolute" right="8px" top="8px" />
    </Alert>
  )
}
```

### useError Hook

```typescript
// src/shared/hooks/useError.ts
export function useError() {
  return useContext(ErrorContext)
}
```

---

## Routing

### Router Setup

```typescript
// src/App.tsx
import { Routes, Route, Navigate } from 'react-router-dom'
import { ProtectedRoute } from '@shared/components/ProtectedRoute'

function App() {
  return (
    <Routes>
      {/* Public routes */}
      <Route path="/login" element={<LoginPage />} />
      <Route path="/register" element={<RegisterPage />} />
      
      {/* Protected routes */}
      <Route
        path="/"
        element={
          <ProtectedRoute>
            <Layout />
          </ProtectedRoute>
        }
      >
        <Route index element={<DashboardPage />} />
        <Route path="machines" element={<MachinesPage />} />
        <Route path="machines/:id" element={<MachineDetailPage />} />
        <Route path="containers" element={<ContainersPage />} />
        <Route path="images" element={<ImagesPage />} />
        <Route path="settings" element={<SettingsPage />} />
      </Route>
      
      {/* Special pages */}
      <Route path="/offline" element={<OfflinePage />} />
      <Route path="*" element={<NotFoundPage />} />
    </Routes>
  )
}
```

### Protected Route Component

```typescript
// src/shared/components/ProtectedRoute.tsx
import { Navigate, useLocation } from 'react-router-dom'
import { useAuth } from '@features/auth/hooks/useAuth'
import { LoadingSpinner } from './LoadingSpinner'

interface ProtectedRouteProps {
  children: React.ReactNode
}

export function ProtectedRoute({ children }: ProtectedRouteProps) {
  const { isAuthenticated, isLoading } = useAuth()
  const location = useLocation()

  if (isLoading) {
    return <LoadingSpinner />
  }

  if (!isAuthenticated) {
    return <Navigate to="/login" state={{ from: location }} replace />
  }

  return <>{children}</>
}
```

### Navigation

```typescript
// Example navigation in components
import { useNavigate } from 'react-router-dom'

function SomeComponent() {
  const navigate = useNavigate()
  
  const goToMachine = (machineId: string) => {
    navigate(`/machines/${machineId}`)
  }
  
  return <Button onClick={() => goToMachine('abc-123')}>View Machine</Button>
}
```

---

## API Service Layer

### Base API Configuration

```typescript
// src/shared/utils/api.ts
import axios, { AxiosError, AxiosInstance, AxiosResponse, InternalAxiosRequestConfig } from 'axios'

const api: AxiosInstance = axios.create({
  baseURL: import.meta.env.VITE_API_BASE_URL,
  withCredentials: true,
  timeout: 30000, // 30 seconds
})

// Request interceptor for adding request ID
api.interceptors.request.use((config: InternalAxiosRequestConfig) => {
  // Request ID can be added here if needed for frontend tracking
  return config
})

// Response interceptor for error handling
api.interceptors.response.use(
  (response: AxiosResponse) => response,
  (error: AxiosError) => {
    if (error.response?.status === 401) {
      window.location.href = '/login'
    }
    return Promise.reject(error)
  }
)

export { api }
```

### Domain-Specific API Services

#### Machines API

```typescript
// src/features/machines/services/machinesApi.ts
import { api } from '@shared/utils/api'

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
  createdAt: string
  updatedAt: string
}

export interface CreateMachineRequest {
  name: string
  description?: string
  hostname: string
  port?: number
  sshUsername: string
  sshPrivateKey: string
  podmanSocketPath?: string
}

export interface ShareMachineRequest {
  userId: string
  permission: 'read' | 'write' | 'admin'
}

export const machinesApi = {
  listMachines: async (): Promise<Machine[]> => {
    const response = await api.get('/machines')
    return response.data
  },

  getMachine: async (machineId: string): Promise<Machine> => {
    const response = await api.get(`/machines/${machineId}`)
    return response.data
  },

  createMachine: async (data: CreateMachineRequest): Promise<Machine> => {
    const response = await api.post('/machines', data)
    return response.data
  },

  updateMachine: async (machineId: string, data: Partial<CreateMachineRequest>): Promise<Machine> => {
    const response = await api.put(`/machines/${machineId}`, data)
    return response.data
  },

  deleteMachine: async (machineId: string): Promise<void> => {
    await api.delete(`/machines/${machineId}`)
  },

  listAccess: async (machineId: string): Promise<Array<{ userId: string; permission: string }>> => {
    const response = await api.get(`/machines/${machineId}/access`)
    return response.data
  },

  grantAccess: async (machineId: string, data: ShareMachineRequest): Promise<void> => {
    await api.post(`/machines/${machineId}/access`, data)
  },

  revokeAccess: async (machineId: string, userId: string): Promise<void> => {
    await api.delete(`/machines/${machineId}/access/${userId}`)
  },
}
```

#### Containers API

```typescript
// src/features/containers/services/containersApi.ts
import { api } from '@shared/utils/api'

export interface Container {
  id: string
  name: string
  state: 'created' | 'running' | 'paused' | 'stopped' | 'removing' | 'exited' | 'dead'
  image: string
  createdAt: string
  // ... other container properties from Podman API
}

export interface CreateContainerRequest {
  name: string
  image: string
  command?: string[]
  // ... other create options
}

export const containersApi = {
  list: async (machineId: string): Promise<Container[]> => {
    const response = await api.get(`/machines/${machineId}/containers`)
    return response.data
  },

  get: async (machineId: string, containerId: string): Promise<Container> => {
    const response = await api.get(`/machines/${machineId}/containers/${containerId}`)
    return response.data
  },

  create: async (machineId: string, data: CreateContainerRequest): Promise<Container> => {
    const response = await api.post(`/machines/${machineId}/containers`, data)
    return response.data
  },

  start: async (machineId: string, containerId: string): Promise<void> => {
    await api.post(`/machines/${machineId}/containers/${containerId}/start`)
  },

  stop: async (machineId: string, containerId: string): Promise<void> => {
    await api.post(`/machines/${machineId}/containers/${containerId}/stop`)
  },

  delete: async (machineId: string, containerId: string): Promise<void> => {
    await api.delete(`/machines/${machineId}/containers/${containerId}`)
  },
}
```

#### Images API

```typescript
// src/features/images/services/imagesApi.ts
import { api } from '@shared/utils/api'

export interface Image {
  id: string
  repoTags: string[]
  repoDigests: string[]
  size: number
  createdAt: string
  // ... other image properties from Podman API
}

export interface PullImageRequest {
  image: string
  // ... pull options
}

export const imagesApi = {
  list: async (machineId: string): Promise<Image[]> => {
    const response = await api.get(`/machines/${machineId}/images`)
    return response.data
  },

  pull: async (
    machineId: string, 
    data: PullImageRequest,
    onProgress?: (progress: ProgressEvent) => void
  ): Promise<Image> => {
    // For streaming, we need special handling
    // See Streaming section below
    throw new Error('Use streaming pull instead')
  },

  delete: async (machineId: string, imageId: string): Promise<void> => {
    await api.delete(`/machines/${machineId}/images/${imageId}`)
  },
}
```

### Quadlets API

```typescript
// src/features/quadlets/services/quadletsApi.ts
import { api } from '@shared/utils/api'

export type QuadletResourceType = 'container' | 'pod' | 'image' | 'volume' | 'network'
export type QuadletState = 'unknown' | 'stopped' | 'running' | 'error'

export interface Quadlet {
  id: string
  name: string
  path: string
  unit: string
  resource: QuadletResourceType
  state: QuadletState
  content?: string
  generation?: number
  createdAt: string
  updatedAt?: string
}

export interface CreateQuadletRequest {
  name: string
  resource: QuadletResourceType
  content: string
}

export interface UpdateQuadletRequest {
  content: string
}

export interface GenerateQuadletRequest {
  resourceId: string
  resourceType: 'container' | 'pod'
}

export interface ImportQuadletRequest {
  path: string
  recursive?: boolean
}

export interface QuadletLogs {
  id: string
  logs: string
}

export const quadletsApi = {
  list: async (machineId: string): Promise<Quadlet[]> => {
    const response = await api.get(`/machines/${machineId}/quadlets`)
    return response.data
  },

  get: async (machineId: string, quadletId: string): Promise<Quadlet> => {
    const response = await api.get(`/machines/${machineId}/quadlets/${quadletId}`)
    return response.data
  },

  create: async (machineId: string, data: CreateQuadletRequest): Promise<Quadlet> => {
    const response = await api.post(`/machines/${machineId}/quadlets`, data)
    return response.data
  },

  update: async (machineId: string, quadletId: string, data: UpdateQuadletRequest): Promise<Quadlet> => {
    const response = await api.put(`/machines/${machineId}/quadlets/${quadletId}`, data)
    return response.data
  },

  delete: async (machineId: string, quadletId: string, query?: { stop?: boolean; force?: boolean }): Promise<void> => {
    await api.delete(`/machines/${machineId}/quadlets/${quadletId}`, { params: query })
  },

  start: async (machineId: string, quadletId: string): Promise<Quadlet> => {
    const response = await api.post(`/machines/${machineId}/quadlets/${quadletId}/start`)
    return response.data
  },

  stop: async (machineId: string, quadletId: string, query?: { force?: boolean; timeout?: number }): Promise<Quadlet> => {
    const response = await api.post(`/machines/${machineId}/quadlets/${quadletId}/stop`, null, { params: query })
    return response.data
  },

  restart: async (machineId: string, quadletId: string, query?: { force?: boolean; timeout?: number }): Promise<Quadlet> => {
    const response = await api.post(`/machines/${machineId}/quadlets/${quadletId}/restart`, null, { params: query })
    return response.data
  },

  getLogs: async (machineId: string, quadletId: string, query?: { follow?: boolean; since?: string; tail?: number }): Promise<QuadletLogs> => {
    const response = await api.get(`/machines/${machineId}/quadlets/${quadletId}/logs`, { params: query })
    return response.data
  },

  generate: async (machineId: string, quadletId: string, data: GenerateQuadletRequest): Promise<Quadlet> => {
    const response = await api.post(`/machines/${machineId}/quadlets/${quadletId}/generate`, data)
    return response.data
  },

  import: async (machineId: string, data: ImportQuadletRequest): Promise<{ imported: Quadlet[]; failed: Array<{ path: string; error: string }> }> => {
    const response = await api.post(`/machines/${machineId}/quadlets/import`, data)
    return response.data
  },
}
```

---

## Streaming (Image Pull Progress)

### Custom Hook for Streaming

```typescript
// src/features/images/hooks/usePullImage.ts
import { useState, useCallback } from 'react'
import { useError } from '@shared/hooks/useError'

interface ProgressEvent {
  operationId: string
  type: 'progress' | 'complete' | 'error'
  machineId: string
  state?: {
    current: number
    total: number
    status: string
    message: string
  }
  timestamp: string
  error?: {
    code: string
    message: string
  }
}

interface UsePullImageResult {
  pullImage: (machineId: string, image: string) => Promise<void>
  progress: ProgressEvent | null
  isPulling: boolean
  abort: () => void
}

export function usePullImage(): UsePullImageResult {
  const [progress, setProgress] = useState<ProgressEvent | null>(null)
  const [isPulling, setIsPulling] = useState(false)
  const [abortController, setAbortController] = useState<AbortController | null>(null)
  const { showError } = useError()

  const abort = useCallback(() => {
    if (abortController) {
      abortController.abort()
      setAbortController(null)
    }
  }, [abortController])

  const pullImage = useCallback(
    async (machineId: string, image: string) => {
      abort() // Cancel any existing pull
      
      setIsPulling(true)
      setProgress(null)
      
      const controller = new AbortController()
      setAbortController(controller)
      
      try {
        const response = await fetch(
          `/api/machines/${machineId}/images/pull`,
          {
            method: 'POST',
            headers: {
              'Content-Type': 'application/json',
            },
            body: JSON.stringify({ image }),
            credentials: 'include',
            signal: controller.signal,
          }
        )

        if (!response.ok) {
          throw new Error(`HTTP error! status: ${response.status}`)
        }

        if (!response.body) {
          throw new Error('No response body')
        }

        const reader = response.body.getReader()
        const decoder = new TextDecoder()

        while (true) {
          const { done, value } = await reader.read()
          
          if (done) {
            break
          }
          
          const text = decoder.decode(value, { stream: true })
          const lines = text.split('\n').filter(line => line.trim())
          
          for (const line of lines) {
            try {
              const event = JSON.parse(line) as ProgressEvent
              setProgress(event)
              
              if (event.type === 'error') {
                showError({ message: event.error?.message || 'Unknown error', code: event.error?.code || 'UNKNOWN_ERROR' })
              } else if (event.type === 'complete') {
                setIsPulling(false)
              }
            } catch (e) {
              console.error('Error parsing progress event:', e, line)
            }
          }
        }
      } catch (error) {
        if (error.name !== 'AbortError') {
          showError({ message: 'Failed to pull image', code: 'IMAGE_PULL_FAILED' })
        }
        setIsPulling(false)
        setProgress(null)
      } finally {
        setAbortController(null)
      }
    },
    [showError]
  )

  return {
    pullImage,
    progress,
    isPulling,
    abort,
  }
}
```

### Streaming with Abort Controller

The streaming implementation uses the Fetch API with `AbortController` to:
1. Open a streaming connection to the backend
2. Read the NDJSON (newline-delimited JSON) response
3. Parse each line as a progress event
4. update local state with progress
5. Handle abort requests from the user
6. Handle connection errors

---

## SSE (Server-Sent Events)

### Centralized Event Bus

```typescript
// src/features/events/eventBus.ts
import { useEffect, useState } from 'react'

interface EventListener {
  (event: any): void
}

interface EventBus {
  subscribe: (machineId: string, eventType: string, listener: EventListener) => () => void
  unsubscribe: (machineId: string, eventType: string, listener: EventListener) => void
  publish: (machineId: string, eventType: string, data: any) => void
}

const listeners: Map<string, Map<string, Set<EventListener>>> = new Map()

const eventBus: EventBus = {
  subscribe: (machineId: string, eventType: string, listener: EventListener) => {
    if (!listeners.has(machineId)) {
      listeners.set(machineId, new Map())
    }
    const machineListeners = listeners.get(machineId)!
    
    if (!machineListeners.has(eventType)) {
      machineListeners.set(eventType, new Set())
    }
    
    const typeListeners = machineListeners.get(eventType)!
    typeListeners.add(listener)
    
    // If this is the first listener for this machine, open the connection
    if (typeListeners.size === 1) {
      openConnection(machineId)
    }
    
    return () => {
      typeListeners.delete(listener)
      if (typeListeners.size === 0) {
        machineListeners.delete(eventType)
      }
      if (machineListeners.size === 0) {
        listeners.delete(machineId)
        closeConnection(machineId)
      }
    }
  },

  unsubscribe: (machineId: string, eventType: string, listener: EventListener) => {
    const machineListeners = listeners.get(machineId)
    if (!machineListeners) return
    
    const typeListeners = machineListeners.get(eventType)
    if (!typeListeners) return
    
    typeListeners.delete(listener)
    if (typeListeners.size === 0) {
      machineListeners.delete(eventType)
    }
    if (machineListeners.size === 0) {
      listeners.delete(machineId)
      closeConnection(machineId)
    }
  },

  publish: (machineId: string, eventType: string, data: any) => {
    const machineListeners = listeners.get(machineId)
    if (!machineListeners) return
    
    const typeListeners = machineListeners.get(eventType)
    if (!typeListeners) return
    
    typeListeners.forEach(listener => listener(data))
  },
}

// Connection management
const connections: Map<string, EventSource> = new Map()

function openConnection(machineId: string) {
  const eventSource = new EventSource(`/api/machines/${machineId}/events`)
  
  eventSource.onopen = () => {
    console.log(`SSE connected to machine ${machineId}`)
  }

  eventSource.onmessage = (event) => {
    try {
      const data = JSON.parse(event.data)
      // Publish to all listeners for this machine
      const machineListeners = listeners.get(machineId)
      if (machineListeners) {
        machineListeners.forEach((_, eventType) => {
          eventBus.publish(machineId, eventType, data)
        })
      }
    } catch (e) {
      console.error('Error parsing SSE event:', e)
    }
  }

  eventSource.onerror = () => {
    console.error(`SSE error for machine ${machineId}`)
    eventSource.close()
    connections.delete(machineId)
    
    // Attempt to reconnect after delay
    setTimeout(() => openConnection(machineId), 5000)
  }
  
  connections.set(machineId, eventSource)
}

function closeConnection(machineId: string) {
  const eventSource = connections.get(machineId)
  if (eventSource) {
    eventSource.close()
    connections.delete(machineId)
  }
}

export { eventBus }
```

### useEvents Hook

```typescript
// src/features/events/hooks/useEvents.ts
import { useEffect } from 'react'
import { eventBus } from '../eventBus'

export function useEvents(machineId: string, eventType: string, callback: (data: any) => void) {
  useEffect(() => {
    const unsubscribe = eventBus.subscribe(machineId, eventType, callback)
    return unsubscribe
  }, [machineId, eventType, callback])
}

// Specific event types
export function useContainerEvents(machineId: string, callback: (data: any) => void) {
  return useEvents(machineId, 'container', callback)
}

export function useImageEvents(machineId: string, callback: (data: any) => void) {
  return useEvents(machineId, 'image', callback)
}

export function usePodmanEvents(machineId: string, callback: (data: any) => void) {
  return useEvents(machineId, 'event', callback)
}
```

### Event Types

```typescript
// src/features/events/types.ts
export interface ContainerEvent {
  type: 'start' | 'stop' | 'create' | 'remove' | 'die' | 'pause' | 'unpause'
  actor: {
    ID: string
    Attributes: Record<string, string>
  }
  time: string
  timeNano: string
}

export interface ImageEvent {
  type: 'pull' | 'push' | 'remove' | 'import' | 'build' | 'save'
  actor: {
    ID: string
    Attributes: Record<string, string>
  }
  time: string
  timeNano: string
}
```

---

## Offline Handling

### Offline Detection

```typescript
// src/shared/hooks/useOffline.ts
import { useEffect, useState } from 'react'

export function useOffline() {
  const [isOffline, setIsOffline] = useState<boolean>(!navigator.onLine)

  useEffect(() => {
    const handleOnline = () => setIsOffline(false)
    const handleOffline = () => setIsOffline(true)

    window.addEventListener('online', handleOnline)
    window.addEventListener('offline', handleOffline)

    // Set up heartbeat
    const heartbeatInterval = setInterval(() => {
      checkBackendConnectivity()
        .then((online) => {
          if (!online && navigator.onLine) {
            // Network is up but backend is down
            setIsOffline(true)
          } else if (online && isOffline) {
            setIsOffline(false)
          }
        })
        .catch(() => {
          setIsOffline(true)
        })
    }, 30000) // Check every 30 seconds

    return () => {
      window.removeEventListener('online', handleOnline)
      window.removeEventListener('offline', handleOffline)
      clearInterval(heartbeatInterval)
    }
  }, [isOffline])

  return isOffline
}

// Check if backend is responsive
async function checkBackendConnectivity(): Promise<boolean> {
  try {
    const controller = new AbortController()
    const timeout = setTimeout(() => controller.abort(), 5000) // 5 second timeout
    
    const response = await fetch('/api/health', {
      method: 'GET',
      signal: controller.signal,
      credentials: 'include',
    })
    
    clearTimeout(timeout)
    return response.ok
  } catch {
    return false
  }
}
```

### Offline page

```typescript
// src/pages/OfflinePage.tsx
import { useEffect } from 'react'
import { useNavigate } from 'react-router-dom'
import { useOffline } from '@shared/hooks/useOffline'
import { Box, Heading, Text, Button, Center, Spinner } from '@chakra-ui/react'

export function OfflinePage() {
  const isOffline = useOffline()
  const navigate = useNavigate()

  useEffect(() => {
    if (!isOffline) {
      navigate('/')
    }
  }, [isOffline, navigate])

  return (
    <Center minH="100vh" p={4}>
      <Box textAlign="center">
        <Heading mb={4}>Offline</Heading>
        <Text mb={8}>You are not connected to the internet or the server is unavailable.</Text>
        <Button onClick={() => window.location.reload()}>
          <Spinner size="sm" mr={2} />
          Try Again
        </Button>
      </Box>
    </Center>
  )
}
```

### Offline Banner

```typescript
// src/shared/components/OfflineBanner.tsx
import { useEffect } from 'react'
import { useOffline } from '@shared/hooks/useOffline'
import { Alert, AlertIcon, AlertTitle, AlertDescription } from '@chakra-ui/react'

export function OfflineBanner() {
  const isOffline = useOffline()

  if (!isOffline) {
    return null
  }

  return (
    <Alert status="warning" position="fixed" bottom="20px" right="20px" width="300px">
      <AlertIcon />
      <Box flex="1">
        <AlertTitle>Offline</AlertTitle>
        <AlertDescription display="block">
          You are currently offline. Changes will not be saved.
        </AlertDescription>
      </Box>
    </Alert>
  )
}
```

---

## Feature Components

### Machines Feature

#### Machine List

```typescript
// src/features/machines/components/MachineList.tsx
import { useMachines } from '../hooks/useMachines'
import { LoadingSpinner } from '@shared/components/LoadingSpinner'
import { ErrorDisplay } from '@shared/components/ErrorDisplay'
import { MachineCard } from './MachineCard'
import { SimpleGrid, Text } from '@chakra-ui/react'

export function MachineList() {
  const { data: machines, isLoading, isError, error } = useMachines()

  if (isLoading) {
    return <LoadingSpinner />
  }

  if (isError) {
    return <ErrorDisplay message={error.message} code={error.code} onClose={() => {}} />
  }

  if (!machines || machines.length === 0) {
    return <Text>No machines found. Add your first machine to get started.</Text>
  }

  return (
    <SimpleGrid columns={{ base: 1, md: 2, lg: 3 }} spacing={6}>
      {machines.map((machine) => (
        <MachineCard key={machine.id} machine={machine} />
      ))}
    </SimpleGrid>
  )
}
```

#### Machine Card

```typescript
// src/features/machines/components/MachineCard.tsx
import { Card, CardHeader, CardBody, CardFooter, Heading, Text, Button, Badge, useDisclosure } from '@chakra-ui/react'
import { Machine } from '../types'
import { useDeleteMachine } from '../hooks/useDeleteMachine'

export function MachineCard({ machine }: { machine: Machine }) {
  const { isOpen, onOpen, onClose } = useDisclosure()
  const deleteMachine = useDeleteMachine()

  const handleDelete = () => {
    deleteMachine.mutate(machine.id)
  }

  return (
    <Card>
      <CardHeader>
        <Heading size="md">{machine.name}</Heading>
        <Badge colorScheme="blue">{machine.permission}</Badge>
      </CardHeader>
      <CardBody>
        <Text noOfLines={2}>{machine.description || 'No description'}</Text>
        <Text fontSize="sm" color="gray.500" mt={2}>
          {machine.hostname}:
          {machine.port}
        </Text>
      </CardBody>
      <CardFooter justify="space-between" flexWrap="wrap">
        <Button onClick={() => onOpen()}>View</Button>
        <Button colorScheme="red" onClick={handleDelete} isLoading={false}>
          Delete
        </Button>
      </CardFooter>
    </Card>
  )
}
```

#### Machine Form

```typescript
// src/features/machines/components/MachineForm.tsx
import { useForm } from 'react-hook-form'
import { Button, FormControl, FormLabel, Input, Textarea, VStack } from '@chakra-ui/react'
import { CreateMachineRequest } from '../types'
import { useCreateMachine } from '../hooks/useCreateMachine'

type FormData = Omit<CreateMachineRequest, 'sshPrivateKey'> & {
  sshPrivateKey: FileList
}

export function MachineForm({ onSuccess }: { onSuccess: () => void }) {
  const { register, handleSubmit, formState: { isSubmitting, errors } } = useForm<FormData>()
  const createMachine = useCreateMachine()

  const onSubmit = async (data: FormData) => {
    const sshKey = await data.sshPrivateKey[0].text()
    
    createMachine.mutate(
      {
        ...data,
        sshPrivateKey: sshKey,
      },
      {
        onSuccess,
      }
    )
  }

  return (
    <form onSubmit={handleSubmit(onSubmit)}>
      <VStack spacing={4}>
        <FormControl isInvalid={!!errors.name}>
          <FormLabel>Name</FormLabel>
          <Input
            {...register('name', { required: 'Name is required' })}
            placeholder="My Server"
          />
        </FormControl>

        <FormControl>
          <FormLabel>Description</FormLabel>
          <Textarea {...register('description')} placeholder="Optional description" />
        </FormControl>

        <FormControl isInvalid={!!errors.hostname}>
          <FormLabel>Hostname/IP</FormLabel>
          <Input
            {...register('hostname', { required: 'Hostname is required' })}
            placeholder="example.com or 192.168.1.100"
          />
        </FormControl>

        <FormControl>
          <FormLabel>Port</FormLabel>
          <Input
            {...register('port', { valueAsNumber: true })}
            type="number"
            defaultValue={22}
            placeholder="22"
          />
        </FormControl>

        <FormControl isInvalid={!!errors.sshUsername}>
          <FormLabel>SSH Username</FormLabel>
          <Input
            {...register('sshUsername', { required: 'SSH username is required' })}
            placeholder="root or your username"
          />
        </FormControl>

        <FormControl isInvalid={!!errors.sshPrivateKey}>
          <FormLabel>SSH Private Key</FormLabel>
          <Input
            {...register('sshPrivateKey', { required: 'SSH private key is required' })}
            type="file"
            accept=".pem,.key"
          />
        </FormControl>

        <Button
          type="submit"
          colorScheme="blue"
          isLoading={isSubmitting}
          width="full"
        >
          Add Machine
        </Button>
      </VStack>
    </form>
  )
}
```

### Containers Feature

#### Container List

```typescript
// src/features/containers/components/ContainerList.tsx
import { useContainers } from '../hooks/useContainers'
import { ContainerCard } from './ContainerCard'
import { LoadingSpinner } from '@shared/components/LoadingSpinner'
import { ErrorDisplay } from '@shared/components/ErrorDisplay'
import { VStack, Text } from '@chakra-ui/react'

export function ContainerList({ machineId }: { machineId: string }) {
  const { data: containers, isLoading, isError, error } = useContainers(machineId)

  if (isLoading) {
    return <LoadingSpinner />
  }

  if (isError) {
    return <ErrorDisplay message={error.message} code={error.code} onClose={() => {}} />
  }

  if (!containers || containers.length === 0) {
    return <Text>No containers found on this machine.</Text>
  }

  return (
    <VStack spacing={4} align="stretch">
      {containers.map((container) => (
        <ContainerCard
          key={container.id}
          machineId={machineId}
          container={container}
        />
      ))}
    </VStack>
  )
}
```

#### Container Actions

```typescript
// src/features/containers/components/ContainerActions.tsx
import { Button, HStack } from '@chakra-ui/react'
import { Container } from '../types'
import { useStartContainer, useStopContainer } from '../hooks'

export function ContainerActions({
  machineId,
  container,
  onActionComplete,
}: {
  machineId: string
  container: Container
  onActionComplete?: () => void
}) {
  const startContainer = useStartContainer(machineId)
  const stopContainer = useStopContainer(machineId)

  const handleStart = () => {
    startContainer.mutate(container.id, {
      onSuccess: onActionComplete,
    })
  }

  const handleStop = () => {
    stopContainer.mutate(container.id, {
      onSuccess: onActionComplete,
    })
  }

  return (
    <HStack>
      {container.state === 'running' ? (
        <Button onClick={handleStop} isLoading={stopContainer.isPending}>
          Stop
        </Button>
      ) : (
        <Button onClick={handleStart} isLoading={startContainer.isPending} colorScheme="green">
          Start
        </Button>
      )}
      <Button colorScheme="red">Delete</Button>
    </HStack>
  )
}
```

### Images Feature

#### Image Pull with Progress

```typescript
// src/features/images/components/ImagePullForm.tsx
import { useState } from 'react'
import { Button, Input, VStack, Progress, Text } from '@chakra-ui/react'
import { usePullImage } from '../hooks/usePullImage'

export function ImagePullForm({ machineId, onComplete }: { machineId: string; onComplete?: () => void }) {
  const [imageName, setImageName] = useState('')
  const { pullImage, progress, isPulling, abort } = usePullImage()

  const handleSubmit = async () => {
    if (!imageName) return
    
    try {
      await pullImage(machineId, imageName)
      onComplete?.()
    } catch (error) {
      // Error is handled by usePullImage
    }
  }

  return (
    <VStack spacing={4}>
      <Input
        value={imageName}
        onChange={(e) => setImageName(e.target.value)}
        placeholder="nginx:latest"
      />
      
      {progress && progress.type === 'progress' && (
        <VStack width="full">
          <Text>{progress.state?.message || `Pulling ${imageName}...`}</Text>
          <Progress
            value={progress.state?.current ?? 0}
            max={progress.state?.total ?? 100}
            size="sm"
            width="full"
            isAnimated
          />
          <Text fontSize="xs">
            {progress.state?.current ?? 0} / {progress.state?.total ?? 100}
          </Text>
        </VStack>
      )}
      
      <Button
        onClick={handleSubmit}
        isLoading={isPulling}
        isDisabled={!imageName || isPulling}
        colorScheme="blue"
        width="full"
      >
        {isPulling ? 'Pulling...' : 'Pull Image'}
      </Button>
      
      {isPulling && (
        <Button onClick={abort} colorScheme="red" variant="outline" width="full">
          Cancel
        </Button>
      )}
    </VStack>
  )
}
```

---

## Pages

### Dashboard Page

```typescript
// src/pages/DashboardPage.tsx
import { useMachines } from '@features/machines/hooks/useMachines'
import { LoadingSpinner } from '@shared/components/LoadingSpinner'
import { ErrorDisplay } from '@shared/components/ErrorDisplay'
import { useAuth } from '@features/auth/hooks/useAuth'
import { Heading, VStack, Text } from '@chakra-ui/react'

export function DashboardPage() {
  const { user } = useAuth()
  const { data: machines, isLoading, isError, error } = useMachines()

  if (isLoading) {
    return <LoadingSpinner />
  }

  if (isError) {
    return <ErrorDisplay message={error.message} code={error.code} onClose={() => {}} />
  }

  return (
    <VStack spacing={8} align="stretch">
      <VStack align="stretch" spacing={4}>
        <Heading>Welcome back, {user?.username}!</Heading>
        <Text>Manage your Podman instances from anywhere.</Text>
      </VStack>

      <VStack align="stretch" spacing={6}>
        <Heading size="md">Your Machines</Heading>
        {/* Machine list or "add your first machine" CTA */}
      </VStack>
    </VStack>
  )
}
```

### Login Page

```typescript
// src/pages/LoginPage.tsx
import { useEffect } from 'react'
import { useNavigate } from 'react-router-dom'
import { useAuth } from '@features/auth/hooks/useAuth'
import { LoginForm } from '@features/auth/components/LoginForm'
import { Box, Heading, Text, Center } from '@chakra-ui/react'

export function LoginPage() {
  const { isAuthenticated, isLoading } = useAuth()
  const navigate = useNavigate()

  useEffect(() => {
    if (!isLoading && isAuthenticated) {
      navigate('/')
    }
  }, [isAuthenticated, isLoading, navigate])

  return (
    <Center minH="100vh" p={4}>
      <Box width="full" maxWidth="md">
        <VStack spacing={8}>
          <VStack>
            <Heading>Welcome to Podland</Heading>
            <Text>Manage Podman on remote machines</Text>
          </VStack>
          
          <LoginForm />
          
          <Text color="gray.500">
            Don't have an account? Contact administrator.
          </Text>
        </VStack>
      </Box>
    </Center>
  )
}
```

### Machines Page

```typescript
// src/pages/MachinesPage.tsx
import { useState } from 'react'
import { MachineList } from '@features/machines/components/MachineList'
import { MachineForm } from '@features/machines/components/MachineForm'
import { useDisclosure, Button, Modal, ModalOverlay, ModalContent, ModalHeader, ModalCloseButton, ModalBody, ModalFooter, Heading } from '@chakra-ui/react'

export function MachinesPage() {
  const { isOpen, onOpen, onClose } = useDisclosure()
  const [refreshKey, setRefreshKey] = useState(0)

  const handleSuccess = () => {
    onClose()
    setRefreshKey(prev => prev + 1)
  }

  return (
    <>
      <Heading mb={6}>Machines</Heading>
      <Button onClick={onOpen} colorScheme="blue" mb={6}>
        Add Machine
      </Button>
      
      <MachineList key={refreshKey} />
      
      <Modal isOpen={isOpen} onClose={onClose} size="xl">
        <ModalOverlay />
        <ModalContent>
          <ModalHeader>Add New Machine</ModalHeader>
          <ModalCloseButton />
          <ModalBody>
            <MachineForm onSuccess={handleSuccess} />
          </ModalBody>
        </ModalContent>
      </Modal>
    </>
  )
}
```

### Machine Detail Page

```typescript
// src/pages/MachineDetailPage.tsx
import { useParams } from 'react-router-dom'
import { useMachine } from '@features/machines/hooks/useMachine'
import { ContainerList } from '@features/containers/components/ContainerList'
import { ImageList } from '@features/images/components/ImageList'
import { QuadletList } from '@features/quadlets/components/QuadletList'
import { LoadingSpinner } from '@shared/components/LoadingSpinner'
import { ErrorDisplay } from '@shared/components/ErrorDisplay'
import { Tabs, TabList, TabPanels, Tab, TabPanel, Heading, Text } from '@chakra-ui/react'

export function MachineDetailPage() {
  const { id } = useParams<{ id: string }>()
  const { data: machine, isLoading, isError, error } = useMachine(id || '')

  if (isLoading) {
    return <LoadingSpinner />
  }

  if (isError) {
    return <ErrorDisplay message={error.message} code={error.code} onClose={() => {}} />
  }

  if (!machine) {
    return <ErrorDisplay message="Machine not found" code="NOT_FOUND" onClose={() => {}} />
  }

  return (
    <>
      <Heading mb={6}>{machine.name}</Heading>
      
      <Tabs variant="enclosed" colorScheme="blue">
        <TabList>
          <Tab>Containers</Tab>
          <Tab>Images</Tab>
          <Tab>Quadlets</Tab>
          <Tab>Settings</Tab>
        </TabList>
        
        <TabPanels>
          <TabPanel>
            <ContainerList machineId={machine.id} />
          </TabPanel>
          <TabPanel>
            <ImageList machineId={machine.id} />
          </TabPanel>
          <TabPanel>
            <QuadletList machineId={machine.id} />
          </TabPanel>
          <TabPanel>
            <Text>Machine settings and sharing</Text>
          </TabPanel>
        </TabPanels>
      </Tabs>
    </>
  )
}
```

---

## Layout Components

### Root Layout

```typescript
// src/shared/components/Layout/index.tsx
import { Outlet } from 'react-router-dom'
import { Box, Flex } from '@chakra-ui/react'
import { Header } from './Header'
import { Sidebar } from './Sidebar'
import { OfflineBanner } from '../OfflineBanner'

export function Layout() {
  return (
    <Flex minH="100vh">
      <Sidebar />
      <Box flex="1">
        <Header />
        <Box p={6} as="main">
          <Outlet />
        </Box>
      </Box>
      <OfflineBanner />
    </Flex>
  )
}
```

### Header

```typescript
// src/shared/components/Layout/Header.tsx
import { Flex, Heading, Spacer, Button, HStack } from '@chakra-ui/react'
import { useAuth } from '@features/auth/hooks/useAuth'
import { ColorModeToggle } from '@shared/components/ColorModeToggle'

export function Header() {
  const { user, logout } = useAuth()

  return (
    <Flex as="header" p={4} borderBottomWidth="1px" align="center">
      <Heading size="md">Podland</Heading>
      <Spacer />
      <HStack>
        <ColorModeToggle />
        {user && (
          <>
            <Button variant="ghost">{user.username}</Button>
            <Button onClick={logout}>Logout</Button>
          </>
        )}
      </HStack>
    </Flex>
  )
}
```

### Sidebar

```typescript
// src/shared/components/Layout/Sidebar.tsx
import { Box, VStack, NavLink, Text } from '@chakra-ui/react'
import { Link, useLocation } from 'react-router-dom'

export function Sidebar() {
  const location = useLocation()

  return (
    <Box as="nav" w="250px" p={4} borderRightWidth="1px" minH="100vh">
      <VStack spacing={4} align="stretch">
        <Text fontWeight="bold" fontSize="lg">Navigation</Text>
        
        <NavLink
          as={Link}
          to="/"
          isActive={location.pathname === '/'}
        >
          Dashboard
        </NavLink>
        
        <NavLink
          as={Link}
          to="/machines"
          isActive={location.pathname.startsWith('/machines')}
        >
          Machines
        </NavLink>
        
        <NavLink
          as={Link}
          to="/settings"
          isActive={location.pathname === '/settings'}
        >
          Settings
        </NavLink>
      </VStack>
    </Box>
  )
}
```

### Color Mode Toggle

```typescript
// src/shared/components/ColorModeToggle.tsx
import { Button, useColorMode } from '@chakra-ui/react'

export function ColorModeToggle() {
  const { colorMode, toggleColorMode } = useColorMode()
  return (
    <Button onClick={toggleColorMode} variant="ghost>
      {colorMode === 'light' ? 'Dark' : 'Light'}
    </Button>
  )
}
```

---

## Utility Components

### Loading Spinner

```typescript
// src/shared/components/LoadingSpinner.tsx
import { Spinner, Center } from '@chakra-ui/react'

export function LoadingSpinner() {
  return (
    <Center minH="400px">
      <Spinner size="xl" />
    </Center>
  )
}
```

### Confirmation Modal

```typescript
// src/shared/components/ConfirmationModal.tsx
import { Modal, ModalOverlay, ModalContent, ModalHeader, ModalCloseButton, ModalBody, ModalFooter, Button } from '@chakra-ui/react'

interface ConfirmationModalProps {
  isOpen: boolean
  onClose: () => void
  title: string
  message: string
  confirmText?: string
  cancelText?: string
  onConfirm: () => void
  isLoading?: boolean
}

export function ConfirmationModal({
  isOpen,
  onClose,
  title,
  message,
  confirmText = 'Confirm',
  cancelText = 'Cancel',
  onConfirm,
  isLoading = false,
}: ConfirmationModalProps) {
  return (
    <Modal isOpen={isOpen} onClose={onClose}>
      <ModalOverlay />
      <ModalContent>
        <ModalHeader>{title}</ModalHeader>
        <ModalCloseButton />
        <ModalBody>{message}</ModalBody>
        <ModalFooter>
          <Button mr={3} onClick={onClose} isDisabled={isLoading}>
            {cancelText}
          </Button>
          <Button colorScheme="red" onClick={onConfirm} isLoading={isLoading}>
            {confirmText}
          </Button>
        </ModalFooter>
      </ModalContent>
    </Modal>
  )
}
```

---

## Custom Hooks

### useLocalStorage

```typescript
// src/shared/hooks/useLocalStorage.ts
import { useState, useEffect } from 'react'

export function useLocalStorage<T>(key: string, initialValue: T): [T, (value: T) => void] {
  const [storedValue, setStoredValue] = useState<T>(() => {
    try {
      const item = window.localStorage.getItem(key)
      return item ? JSON.parse(item) : initialValue
    } catch {
      return initialValue
    }
  })

  const setValue = (value: T) => {
    try {
      setStoredValue(value)
      window.localStorage.setItem(key, JSON.stringify(value))
    } catch {
      // Ignore errors (e.g., in SSR)
    }
  }

  return [storedValue, setValue]
}
```

### useDebounce

```typescript
// src/shared/hooks/useDebounce.ts
import { useState, useEffect } from 'react'

export function useDebounce<T>(value: T, delay: number): T {
  const [debouncedValue, setDebouncedValue] = useState<T>(value)

  useEffect(() => {
    const handler = setTimeout(() => {
      setDebouncedValue(value)
    }, delay)

    return () => {
      clearTimeout(handler)
    }
  }, [value, delay])

  return debouncedValue
}
```

---

## Main Entry Point

```typescript
// src/main.tsx
import React from 'react'
import ReactDOM from 'react-dom/client'
import { BrowserRouter } from 'react-router-dom'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { ReactQueryDevtools } from '@tanstack/react-query-devtools'
import { ChakraProvider, ColorModeScript } from '@chakra-ui/react'
import { AuthProvider } from './shared/context/AuthContext'
import { ErrorProvider } from './shared/context/ErrorContext'
import { theme } from './theme'
import App from './App'

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      refetchOnWindowFocus: false,
      retry: 1,
      staleTime: 5 * 60 * 1000,
    },
    mutations: {
      retry: 0,
    },
  },
})

ReactDOM.createRoot(document.getElementById('root')!).render(
  <React.StrictMode>
    <ColorModeScript initialColorMode={theme.config.initialColorMode} />
    <ChakraProvider theme={theme}>
      <QueryClientProvider client={queryClient}>
        <BrowserRouter>
          <ErrorProvider>
            <AuthProvider>
              <App />
            </AuthProvider>
          </ErrorProvider>
        </BrowserRouter>
        <ReactQueryDevtools initialIsOpen={false} />
      </QueryClientProvider>
    </ChakraProvider>
  </React.StrictMode>
)
```

---

## Package Dependencies

```json
{
  "name": "podland-frontend",
  "private": true,
  "version": "0.0.0",
  "type": "module",
  "dependencies": {
    "@chakra-ui/react": "^2.8.0",
    "@emotion/react": "^11.11.1",
    "@emotion/styled": "^11.11.0",
    "@tanstack/react-query": "^5.17.0",
    "@tanstack/react-query-devtools": "^5.17.0",
    "framer-motion": "^10.16.16",
    "react": "^18.2.0",
    "react-dom": "^18.2.0",
    "react-hook-form": "^7.49.2",
    "react-router-dom": "^6.21.0"
  },
  "devDependencies": {
    "@types/react": "^18.2.43",
    "@types/react-dom": "^18.2.17",
    "@types/node": "^20.10.6",
    "@vitejs/plugin-react": "^4.2.1",
    "typescript": "^5.3.3",
    "eslint": "^8.55.0",
    "eslint-plugin-react-hooks": "^4.6.0",
    "eslint-plugin-react-refresh": "^0.4.5",
    "prettier": "^3.1.1",
    "vite": "^5.0.8"
  }
}
```

---

## Open Questions for Frontend

1. **Form Validation Library**: Should we use React Hook Form, Formik, or custom validation?
2. **Portal for Hooks**: Should we create a centralized hooks index for easier discovery?
3. **Storybook**: Should we add Storybook for component documentation?
4. **Internationalization**: Should we add i18n support from the beginning?

---

*Generated from comprehensive grilling sessions*
*Last updated: 2026-06-30*
