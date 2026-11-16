import type { Register } from 'claude-code'

// Lumon in a pane beside the chat, in Claude Code. The game runs as its own
// process through bin/lumon-hook (event pane), so the binary is hash-checked
// as for every hook. It prints each changed frame as Raster cells; the pane
// shows them, and the keys strip under it passes the player's keys back
// through a file the game reads each frame. See cmd/lumon/pane.go.
//
// Opening: /lumon:play, /lumon:watch and /lumon:screensaver are answered here,
// so the pane opens at the person's request (placed at any width) and the
// command costs no model turn. With automatic opening enabled, a long wait
// opens it too: while this module runs it touches status/pane.alive, and a hook
// that would open the game leaves status/pane.request instead
// (internal/agent/pane.go). Where Claude Code will not place the pane, the game
// opens in a window as it does without it.

// ---- Constants and types ----

const PANE = 'lumon'

// Requests, not grants: docked beside a fullscreen transcript the pane takes at
// most 45% of the terminal's width; stacked above the prompt, a person's own
// resize wins. Lumon wants room either way.
const OPEN = { id: PANE, title: 'LUMON', columns: 120, rows: 34 } as const

// How old a request may be and still open the pane.
const REQUEST_MAX_AGE_MS = 30_000

// How often the pane says it is alive and looks for a request, in ms.
const POLL_MS = 1000

// The game's own size limits, and the smallest pane it is asked to fill.
const MAX_COLUMNS = 512
const MAX_ROWS = 256
const MIN_COLUMNS = 20
const MIN_ROWS = 8

// The game reads only the newest input events; older ones are dropped.
const MAX_EVENTS = 64

// pane.request starts with the time it was made, in nanoseconds.
const NS_PER_MS = 1e6

// blankCell is one space in the terminal's own colours: [0x20, 0x01000000,
// 0x01000000] as little-endian uint32s, twelve bytes, so sixteen base64
// characters that repeat whole.
const blankCell = 'IAAAAAAAAAEAAAAB'

type Frame = { columns: number; rows: number; cells: string }

// How the game starts: at a desk, watching the computer refiners, or on the
// screensaver. A request in any other mode is play.
type Mode = 'play' | 'watch' | 'screensaver'

type Paths = { root: string; data: string; status: string; input: string; shim: string }

type Key = { key: string; ctrl?: boolean; shift?: boolean }

// What each mode tells the person once the pane is open.
const OPENED: Record<Mode, string> = {
  play: 'Lumon is open beside the chat. Click the strip under it to play; Esc gives the keys back.',
  watch:
    'Lumon is open beside the chat, with computer refiners at work. Click the strip under it for the keys; Esc gives them back.',
  screensaver:
    "Lumon's screensaver is open beside the chat. Click the strip under it and press any key to play; Esc gives the keys back.",
}

// ---- Module state ----

// One pane per Claude Code process; these live as long as the module.
let paths: Paths | undefined
let frame: Frame | undefined
let size = { columns: 0, rows: 0 }
let seq = 0
let events: string[] = []
let running = false
let stop: (() => void) | undefined
let lastRequest = ''

// ---- Helpers ----

const modeOf = (m: string): Mode => (m === 'watch' || m === 'screensaver' ? m : 'play')

// The hooks API is untyped here, so `$` and the options are `any`.

// locate finds the plugin's data folder, which hooks are told and a module is
// not. Claude Code keeps it at plugins/data/<plugin>-<marketplace>, the
// marketplace being where the plugin was installed from (its cache folder),
// or "inline" for a plugin loaded from a folder. The status folder is the one
// every agent's plugin shares, as bin/lumon-hook finds it.
async function locate($: any): Promise<Paths> {
  if (paths) return paths
  const root: string = $.plugin.root
  const home = (await $.env.get('HOME')) ?? ''
  const config = (await $.env.get('CLAUDE_CONFIG_DIR')) || `${home}/.claude`
  const market = root.match(/\/plugins\/cache\/([^/]+)\//)?.[1] ?? 'inline'
  const id = `${$.plugin.name}-${market}`.replace(/[^A-Za-z0-9_-]/g, '-')
  const data = `${config}/plugins/data/${id}`
  const state = (await $.env.get('XDG_STATE_HOME')) || `${home}/.local/state`
  const status = (await $.env.get('LUMON_STATUS_DIR')) || `${state}/lumon/agents`
  paths = { root, data, status, input: `${data}/pane-input`, shim: `${root}/bin/lumon-hook` }
  return paths
}

// shimEnv is what bin/lumon-hook reads, as a hook would be given it.
function shimEnv(p: Paths, options: any, mode: Mode): Record<string, string> {
  return {
    CLAUDE_PLUGIN_ROOT: p.root,
    CLAUDE_PLUGIN_DATA: p.data,
    LUMON_STATUS_DIR: p.status,
    CLAUDE_PLUGIN_OPTION_DELAY: String(options?.delay ?? 'off'),
    CLAUDE_PLUGIN_OPTION_COMPANY: String(options?.company ?? '0'),
    CLAUDE_PLUGIN_OPTION_MODE: mode,
  }
}

// send appends one numbered event to the input file the game reads each frame.
async function send($: any, line: string) {
  if (!paths) return
  seq++
  events = [...events, `${seq} ${line}`].slice(-MAX_EVENTS)
  await $.fs.write(paths.input, events.join('\n') + '\n')
}

// blank is an empty frame of the pane's size, until the game's first arrives.
function blank(columns: number, rows: number): Frame {
  return { columns, rows, cells: blankCell.repeat(columns * rows) }
}

// keyName is the key as the game's Bubble Tea names it, or '' to drop it.
function keyName(d: Key): string {
  const named: Record<string, string> = { return: 'enter', escape: 'esc' }
  let k = named[d.key] ?? d.key
  if (k === ' ') k = 'space'
  if (d.ctrl) return k === 'c' ? '' : 'ctrl+' + k
  if (d.shift && ['up', 'down', 'left', 'right'].includes(k)) return 'shift+' + k
  return k
}

// ---- The game ----

// play starts the game for the pane, which closes when the game ends.
function play($: any, options: any, mode: Mode) {
  if (running || !paths) return
  const p = paths
  running = true
  seq = 0
  events = []
  void (async () => {
    await $.fs.write(p.input, '')
    if (size.columns > 0) await send($, `size ${size.columns} ${size.rows}`)
    const game = $.process.spawn({ argv: ['sh', p.shim, 'pane'], env: shimEnv(p, options, mode) })
    stop = () => void game.return?.(undefined)
    let pending = ''
    let said = ''
    let shown = false
    try {
      for await (const piece of game) {
        if (piece.stream === 'stderr') {
          said += piece.text
          continue
        }
        // Only the newest whole line matters; older frames are already stale.
        pending += piece.text
        let latest = ''
        let nl: number
        while ((nl = pending.indexOf('\n')) >= 0) {
          latest = pending.slice(0, nl)
          pending = pending.slice(nl + 1)
        }
        if (!latest.startsWith('F ')) continue
        const [, w, h, cells] = latest.split(' ')
        const next = { columns: Number(w), rows: Number(h), cells }
        const resized = !frame || frame.columns !== next.columns || frame.rows !== next.rows
        frame = next
        shown = true
        // A new size needs a fresh render; the same size can blit the cells alone.
        if (resized) $.ui.invalidate('ui.render')
        else await $.ui.blit({ requestId: PANE, key: 'game', cells })
      }
    } catch (err) {
      said += String(err)
    } finally {
      running = false
      frame = undefined
      stop = undefined
      if (!shown && said.trim() !== '') $.ui.toast(said.trim().split('\n')[0])
      await $.ui.close({ id: PANE })
    }
  })()
}

// open shows the pane and starts the game, or, where Claude Code does not
// place the pane, opens the game in a window. It says which.
async function open($: any, options: any, mode: Mode): Promise<string> {
  const p = await locate($)
  const opened = await $.ui.open(OPEN)
  if (!opened.isPlaced) {
    await $.ui.close({ id: PANE })
    await $.process.run(['sh', p.shim, 'window'], { env: shimEnv(p, options, mode), stdin: '' })
    return 'Lumon is opening in a window: Claude Code has no room for the pane here.'
  }
  play($, options, mode)
  return OPENED[mode]
}

// A command still answers if opening fails, and a close goes on regardless.
function failed() {
  return { text: 'Lumon could not open here; see claude --debug for why.' }
}
function closeAnyway($: any, e: any, next: any) {
  return next(e)
}

// ---- The plugin ----

/** Registers the pane's commands, its rendering, and the poll that opens it on a hook's request. */
export const register: Register = (on, options) => {
  on('session.start', async ($, e, next) => {
    const started = await next(e)
    const p = await locate($)
    // A request left before this session started is not for it.
    try {
      lastRequest = await $.fs.read(`${p.status}/pane.request`)
    } catch {
      lastRequest = ''
    }
    // Tell the hooks the pane is here, and take what they ask for.
    $.clock.every(POLL_MS, async () => {
      if (!(await $.fs.exists(p.status))) return
      await $.fs.write(`${p.status}/pane.alive`, '')
      let request = ''
      try {
        request = await $.fs.read(`${p.status}/pane.request`)
      } catch {
        return
      }
      if (request === lastRequest) return
      lastRequest = request
      const [at, mode] = request.trim().split(' ')
      if (running || Date.now() - Number(at) / NS_PER_MS > REQUEST_MAX_AGE_MS) return
      await open($, options, modeOf(mode))
    })
    return started
  })

  on('command.run', { command: 'lumon:play' }, async $ => ({
    text: await open($, options, 'play'),
  })).catch(failed)
  on('command.run', { command: 'lumon:watch' }, async $ => ({
    text: await open($, options, 'watch'),
  })).catch(failed)
  on('command.run', { command: 'lumon:screensaver' }, async $ => ({
    text: await open($, options, 'screensaver'),
  })).catch(failed)

  on('ui.close', async ($, e, next) => {
    if (e.id === PANE && running) {
      await send($, 'quit')
      stop?.()
    }
    return next(e)
  }).catch(closeAnyway)

  on('ui.message', async ($, e, next) => {
    if (e.requestId !== PANE || e.element !== 'keys') return next(e)
    const name = keyName(e.data as Key)
    if (name !== '' && running) await send($, `key ${name}`)
    return {}
  })

  on('ui.render', { component: 'Pane', requestId: PANE }, async ($, e) => {
    if (e.surface !== 'terminal') {
      const { Text } = $.ui.resolve(e)
      return <Text>Lumon draws in the terminal; /lumon:play there.</Text>
    }
    const { Box, Raster, Client } = $.ui.resolve(e)
    // One body row is left for the keys strip.
    const columns = Math.max(MIN_COLUMNS, Math.min(MAX_COLUMNS, e.props.bodyColumns))
    const rows = Math.max(MIN_ROWS, Math.min(MAX_ROWS, e.props.scroll.bodyRows - 1))
    if (columns !== size.columns || rows !== size.rows) {
      size = { columns, rows }
      if (running) void send($, `size ${columns} ${rows}`)
    }
    const shown = frame && frame.columns === columns && frame.rows === rows ? frame : blank(columns, rows)
    return (
      <Box flexDirection="column">
        <Raster key="game" columns={shown.columns} rows={shown.rows} cells={shown.cells} />
        <Client
          key="keys"
          module="./keys.tsx"
          props={{ label: '▶ click here for the keys · Esc gives them back to Claude' }}
        />
      </Box>
    )
  })
}
