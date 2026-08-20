import { IconButton, useColorMode } from '@chakra-ui/react'
import { MoonIcon, SunIcon } from '@chakra-ui/icons'

export function ColorModeToggle() {
  const { colorMode, toggleColorMode } = useColorMode()
  const isLight = colorMode === 'light'
  return (
    <IconButton
      aria-label={isLight ? 'Switch to dark mode' : 'Switch to light mode'}
      icon={isLight ? <MoonIcon /> : <SunIcon />}
      onClick={toggleColorMode}
      variant="ghost"
    />
  )
}
