// Client-side commands render the game inside OpenCode, without a model turn.
import { homedir } from "node:os";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { onCleanup, onMount } from "solid-js";
import { Renderable, RGBA, type CliRenderer, type KeyEvent, type OptimizedBuffer } from "@opentui/core";
import type { JSX } from "@opentui/solid";
import type { Plugin as TuiV2 } from "@opencode/plugin/tui";
import type { TuiPluginApi } from "@opencode-ai/plugin/tui";
import { AutoOpen, DEFAULT, InlineGame, modeOf, sendStatus, type Frame, type Mode } from "./inline.js";

// ---- Constants and types ----

const root = dirname(fileURLToPath(import.meta.url));
const dataDir = () => join(process.env.XDG_DATA_HOME || join(homedir(), ".local/share"), "opencode/lumon");
const PAGE = "lumon";

// A cell is three little-endian uint32s: code point, foreground, background.
const CELL_BYTES = 12;

// The game's own size limits, as in inline.ts.
const MAX_COLUMNS = 512;
const MAX_ROWS = 256;

// The title line takes the top row; the game draws below it.
const TITLE_ROWS = 1;

// OpenCode 2 events that start or end an agent's turn.
const V2_EVENTS = [
  "session.execution.started",
  "session.execution.succeeded",
  "session.execution.failed",
  "session.execution.interrupted",
  "permission.asked",
  "session.deleted",
];

// The commands that switch the agent integration, named after the event they send.
const SWITCHES = ["off", "snooze", "on"] as const;

type Command = { name: string; description: string; run(): void };

// Host is what attach needs from OpenCode, so one implementation serves both plugin APIs.
type Host = {
  renderer: CliRenderer;
  options: Record<string, unknown>;
  registerPage(render: () => JSX.Element): () => void;
  current(): unknown;
  navigate(route: unknown): void;
  destination(): unknown;
  mode(): () => void;
  commands(commands: Command[]): () => void;
  events(onEvent: (type: string, session?: string, state?: string) => void): () => void;
  session(route: unknown): string | undefined;
  toast(message: string): void;
};

// ---- The game view ----

/** The game as an OpenTUI renderable, drawing cells into OpenTUI's own framebuffer in full RGB. */
export class LumonRenderable extends Renderable {
  private game: InlineGame;
  private frame?: Frame;
  private colors = new Map<number, RGBA>();
  private fg = RGBA.fromHex("#d0d0d0");
  private bg = RGBA.fromHex("#141414");

  constructor(
    renderer: CliRenderer,
    mode: Mode,
    options: Record<string, unknown>,
    private close: (message?: string) => void,
  ) {
    super(renderer, { id: "lumon-game", width: "100%", height: "100%", buffered: true });
    this.focusable = true;
    const changed = (frame: Frame) => {
      this.frame = frame;
      this.requestRender();
    };
    this.game = new InlineGame(root, dataDir(), changed, message => this.close(message));
    this.game.start(mode, options);
  }

  // color caches one RGBA per colour; frames reuse few colours, cell after cell.
  private color(value: number, background: boolean): RGBA {
    if (value === DEFAULT) return background ? this.bg : this.fg;
    let color = this.colors.get(value);
    if (!color) {
      color = RGBA.fromInts(value >> 16, (value >> 8) & 255, value & 255);
      this.colors.set(value, color);
    }
    return color;
  }

  // The base constructor can resize before this.game is set.
  protected onResize(width: number, height: number): void {
    this.game?.resize(width, Math.max(1, height - TITLE_ROWS));
  }

  protected renderSelf(buffer: OptimizedBuffer): void {
    const columns = Math.min(MAX_COLUMNS, this.width);
    const rows = Math.min(MAX_ROWS, Math.max(1, this.height - TITLE_ROWS));
    this.game.resize(columns, rows);
    buffer.drawText("LUMON · Ctrl+] returns to chat".slice(0, this.width), 0, 0, this.fg, this.bg);
    if (!this.frame || this.frame.columns !== columns || this.frame.rows !== rows) {
      if (this.height > 1) buffer.drawText("Loading Lumon…".slice(0, this.width), 0, 1, this.fg, this.bg);
      return;
    }
    const { cells } = this.frame;
    for (let y = 0; y < rows && y + 1 < this.height; y++) {
      for (let x = 0; x < columns; x++) {
        const at = (y * columns + x) * CELL_BYTES;
        const char = String.fromCodePoint(cells.readUInt32LE(at));
        const fg = this.color(cells.readUInt32LE(at + 4), false);
        const bg = this.color(cells.readUInt32LE(at + 8), true);
        buffer.setCell(x, y + 1, char, fg, bg);
      }
    }
  }

  /** Takes every key while the game is shown; Ctrl+] or Ctrl+C stops it. */
  handleKeyPress(event: KeyEvent): boolean {
    event.preventDefault();
    event.stopPropagation();
    if (event.eventType === "release") return true;
    if (event.ctrl && (event.name === "]" || event.name === "c")) {
      this.game.stop();
      return true;
    }
    let key = event.name;
    if (event.ctrl) key = "ctrl+" + key;
    else if (event.meta || event.option) key = "alt+" + key;
    else if (event.shift && ["up", "down", "left", "right"].includes(key)) key = "shift+" + key;
    else if (event.shift && key.length === 1) key = key.toUpperCase();
    this.game.key(key);
    return true;
  }

  protected destroySelf(): void {
    this.game.stop();
    super.destroySelf();
  }
}

// ---- Commands, page and automatic opening ----

// attach registers the page, commands and events with a host, and returns their cleanup.
function attach(host: Host): () => void {
  let game: LumonRenderable | undefined;
  let previous: unknown;
  let mode: Mode = "play";
  let closed = true;
  let disposing = false;
  let working = false;
  const automatic = new AutoOpen();
  const cleanups: (() => void)[] = [];

  // close leaves the game page for the route it was opened from.
  function close(message?: string): void {
    if (closed) return;
    closed = true;
    game?.destroy();
    game = undefined;
    if (!host.renderer.isDestroyed) host.navigate(previous);
    if (message && !disposing) host.toast(message);
  }

  // open remembers the current route and shows the game page in the given mode.
  function open(next: Mode): void {
    automatic.cancel();
    if (!closed) return;
    previous = host.current();
    mode = next;
    closed = false;
    host.navigate(host.destination());
  }

  // The session the player is in, or was in before the game opened.
  const selectedSession = () => host.session(closed ? host.current() : previous);

  cleanups.push(
    host.registerPage(() => {
      const releaseMode = host.mode();
      const view = new LumonRenderable(host.renderer, mode, host.options, close);
      game = view;
      onMount(() => view.focus());
      onCleanup(() => {
        releaseMode();
        closed = true;
        if (game === view) game = undefined;
        view.destroy();
      });
      return view as unknown as JSX.Element;
    }),
  );

  const switches: Command[] = SWITCHES.map(event => ({
    name: `lumon-${event}`,
    description: `${event === "on" ? "Enable" : "Pause"} Lumon agent integration`,
    run: () => {
      automatic.cancel();
      sendStatus(root, dataDir(), event);
      host.toast(
        event === "on"
          ? "Lumon agent integration enabled; automatic opening still requires a delay."
          : "Lumon agent integration paused. Manual play still works.",
      );
    },
  }));
  cleanups.push(
    host.commands([
      { name: "lumon-play", description: "Play Lumon inside OpenCode", run: () => open("play") },
      {
        name: "lumon-watch",
        description: "Watch computer refiners inside OpenCode",
        run: () => open("watch"),
      },
      {
        name: "lumon-screensaver",
        description: "Show Lumon's screensaver inside OpenCode",
        run: () => open("screensaver"),
      },
      ...switches,
    ]),
  );

  // A turn in the selected session may open the game after a delay; anything
  // else ends the turn. OpenCode 1 reports a turn as a busy status.
  cleanups.push(
    host.events((type, session, state) => {
      const selected = selectedSession();
      if (!selected || session !== selected) return;
      if (type === "session.execution.started" || (type === "session.status" && state === "busy")) {
        if (working) return;
        working = true;
        automatic.prompt(host.options.delay ?? process.env.LUMON_DELAY, () => {
          // The player may have moved to another session meanwhile.
          if (selectedSession() === session) open(modeOf(host.options.mode ?? process.env.LUMON_MODE));
        });
      } else {
        working = false;
        automatic.cancel();
        if (type === "session.deleted") close();
      }
    }),
  );

  return () => {
    disposing = true;
    automatic.cancel();
    close();
    for (const cleanup of cleanups.reverse()) cleanup();
  };
}

// ---- OpenCode 2 ----

// setup attaches through OpenCode 2's router and keymap.
const setup: TuiV2.Definition["setup"] = ctx =>
  attach({
    renderer: ctx.renderer,
    options: ctx.options,
    registerPage: render => ctx.ui.router.register({ name: PAGE, render }),
    current: () => ctx.ui.router.current(),
    navigate: route => ctx.ui.router.navigate(route as Parameters<typeof ctx.ui.router.navigate>[0]),
    destination: () => ({ type: "plugin", name: PAGE }),
    mode: () => ctx.keymap.mode.push("lumon"),
    commands: commands => {
      ctx.keymap.layer(() => ({
        commands: commands.map(command => ({
          id: command.name,
          title: command.description,
          palette: true,
          slash: { name: command.name },
          run: command.run,
        })),
      }));
      return () => {};
    },
    events: handler =>
      ctx.data.listen(({ details: event }) => {
        if (V2_EVENTS.includes(event.type)) {
          handler(event.type, "sessionID" in event.data ? event.data.sessionID : undefined);
        }
      }),
    session: route => {
      const value = route as { type?: string; sessionID?: string } | undefined;
      if (value?.type !== "session") return undefined;
      return (value as { sessionID: string }).sessionID;
    },
    toast: message => ctx.ui.toast.show({ message, variant: "info" }),
  });

// ---- OpenCode 1 ----

// tui attaches through OpenCode 1's route and command APIs, which inline play needs.
async function tui(api: TuiPluginApi, options: Record<string, unknown> = {}): Promise<void> {
  if (!api.command) {
    api.ui.toast({
      message: "Lumon needs OpenCode's TUI command API. Upgrade to OpenCode 2 for inline gameplay.",
      variant: "error",
    });
    return;
  }
  const cleanup = attach({
    renderer: api.renderer,
    options,
    registerPage: render => api.route.register([{ name: PAGE, render }]),
    current: () => api.route.current,
    navigate: route => {
      const value = route as { name: string; params?: Record<string, unknown> };
      api.route.navigate(value.name, value.params);
    },
    destination: () => ({ name: PAGE }),
    mode: () => api.mode?.push("lumon") ?? (() => {}),
    commands: commands =>
      api.command!.register(() =>
        commands.map(command => ({
          value: command.name,
          title: command.description,
          slash: { name: command.name },
          onSelect: dialog => {
            dialog?.clear();
            command.run();
          },
        })),
      ),
    events: handler => {
      const stop = [
        api.event.on("session.status", event =>
          handler(event.type, event.properties.sessionID, event.properties.status.type),
        ),
        api.event.on("session.idle", event => handler(event.type, event.properties.sessionID)),
        api.event.on("permission.asked", event => handler(event.type, event.properties.sessionID)),
        api.event.on("session.deleted", event => handler(event.type, event.properties.info.id)),
      ];
      return () => stop.forEach(dispose => dispose());
    },
    session: route => {
      const value = route as { name?: string; params?: { sessionID?: string } } | undefined;
      if (value?.name !== "session") return undefined;
      return (value as { params: { sessionID: string } }).params.sessionID;
    },
    toast: message => api.ui.toast({ message, variant: "info" }),
  });
  api.lifecycle.onDispose(cleanup);
}

/** Lumon's TUI plugin, for both OpenCode 1 (`tui`) and OpenCode 2 (`setup`). */
export default { id: "lumon", tui, setup };
