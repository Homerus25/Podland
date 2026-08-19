import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { ReactQueryDevtools } from '@tanstack/react-query-devtools'
import { ChakraProvider, ColorModeScript } from '@chakra-ui/react'
import { createRoot } from 'react-dom/client'
import { BrowserRouter } from 'react-router-dom'
import { App } from './App'
import { theme } from './theme'

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

createRoot(document.getElementById('root')!).render(
  <QueryClientProvider client={queryClient}>
    <ColorModeScript initialColorMode={theme.config.initialColorMode} />
    <ChakraProvider theme={theme}>
      <BrowserRouter>
        <App />
      </BrowserRouter>
    </ChakraProvider>
    {import.meta.env.DEV && <ReactQueryDevtools initialIsOpen={false} />}
  </QueryClientProvider>,
)
