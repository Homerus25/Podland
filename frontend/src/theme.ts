import { extendTheme, type ThemeConfig } from '@chakra-ui/react'

const config: ThemeConfig = {
  initialColorMode:
    (import.meta.env.VITE_DEFAULT_THEME as ThemeConfig['initialColorMode']) ||
    'system',
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
