import type { ClientModule } from 'claude-code'

// The strip under the game: clicked, it holds the keyboard and posts every key
// to the hooks module, which passes it to the game. Escape hands the keyboard
// back to the prompt.
const Keys: ClientModule<{ label: string }, { listening: true }> = (props, surface) => {
  if (surface.state === undefined) {
    surface.onKey(k => surface.post({ key: k.key, ctrl: k.ctrl ?? false, shift: k.shift ?? false }))
    surface.setState({ listening: true })
  }
  const { Text } = surface.elements
  return <Text dimColor>{props.label}</Text>
}

export default Keys
