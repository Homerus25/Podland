import { Badge } from '@chakra-ui/react'

// Color palette for container/pod states, defined once so every tab shares
// the same vocabulary: running green, exited gray, paused yellow, dead red.
// Anything unrecognized falls back to a neutral gray badge.
const STATE_COLOR_SCHEMES: Record<string, string> = {
  running: 'green',
  exited: 'gray',
  paused: 'yellow',
  dead: 'red',
  created: 'blue',
  restarting: 'orange',
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
