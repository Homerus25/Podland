import { Badge } from '@chakra-ui/react'

// Color palette for state badges, defined once so every tab shares the same
// vocabulary. Containers: running green, exited gray, paused yellow, dead red.
// Quadlets: running green, stopped gray, error red, unknown falls back to
// neutral gray like every unrecognized value.
const STATE_COLOR_SCHEMES: Record<string, string> = {
  running: 'green',
  exited: 'gray',
  paused: 'yellow',
  dead: 'red',
  created: 'blue',
  restarting: 'orange',
  stopped: 'gray',
  error: 'red',
}

export function stateColorScheme(state: string): string {
  return STATE_COLOR_SCHEMES[state.toLowerCase()] ?? 'gray'
}

export function StateBadge({ state }: { state: string }) {
  return (
    <Badge colorScheme={stateColorScheme(state)} data-testid="state-badge">
      {state}
    </Badge>
  )
}
