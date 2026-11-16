// The same transport ships with each independently installed agent plugin.
// It runs the hash-checked game in pipe mode; it never launches a terminal.
import { spawn, type ChildProcess } from "node:child_process";
import { mkdirSync, mkdtempSync, writeFileSync, renameSync, rmSync, readFileSync, statSync } from "node:fs";
import { join } from "node:path";
import { homedir } from "node:os";

// ---- Types and limits ----

/** How a game starts: at a desk, watching computer refiners, or on the screensaver. */
export type Mode = "play" | "watch" | "screensaver";

/** One decoded frame from the game: `columns` by `rows` cells, row by row. */
export type Frame = { columns: number; rows: number; cells: Buffer };

/** The colour value meaning "the terminal's own colour" in a cell's foreground or background. */
export const DEFAULT = 0x01000000;

// A cell is three little-endian uint32s: code point, foreground, background.
const CELL_BYTES = 12;

// The game never draws a larger screen than this.
const MAX_COLUMNS = 512;
const MAX_ROWS = 256;

// The longest frame line: base64 needs 16 characters per cell, plus the header.
const MAX_LINE = MAX_COLUMNS * MAX_ROWS * 16 + 128;

// The game reads only the newest input events; older ones are dropped.
const MAX_EVENTS = 64;

// The native launchHold (internal/agent/hook.go): a younger launch.lock
// belongs to a game still starting. In ms.
const LAUNCH_LOCK_MS = 15_000;

// The longest automatic opening delay, in seconds.
const MAX_DELAY_SECONDS = 600;

// The shim may spend up to 120s fetching the release on first run, plus margin.
const START_TIMEOUT_MS = 130_000;

// How long a game has to exit after TERM before it is killed, in ms.
const KILL_GRACE_MS = 1000;

// How much of the game's stderr is kept to explain a failed start, in characters.
const MAX_ERROR_OUTPUT = 2048;

// The longest message passed back to the host, in characters.
const MAX_MESSAGE = 240;

// The events the native hook accepts from these plugins. Manual open events
// are absent: automatic opening belongs to the UI, and the hook never delays.
const STATUS_EVENTS = ["start", "prompt", "waiting", "stop", "idle", "end", "tool", "off", "on", "snooze"];

// A key name the game knows: any modifiers, then a named key, letter or digit.
const KNOWN_KEY = new RegExp(
  "^(?:(?:ctrl|shift|alt)\\+)*" +
    "(?:up|down|left|right|enter|esc|space|tab|backspace|delete|home|end|pgup|pgdown|[a-z0-9])$",
);

type Tool = { tool_name: string; tool_input: unknown };

// ---- Pure helpers ----

/** Reads a mode from settings or the environment; anything unknown is play. */
export const modeOf = (mode: unknown): Mode => (mode === "watch" || mode === "screensaver" ? mode : "play");

/** The status folder every agent's plugin shares, as bin/lumon-hook finds it. */
export const statusDir = () =>
  process.env.LUMON_STATUS_DIR ||
  join(process.env.XDG_STATE_HOME || join(homedir(), ".local/state"), "lumon/agents");

/** Decodes one `F columns rows base64` line, or returns undefined for anything malformed or unsafe. */
export function parseFrame(line: string): Frame | undefined {
  const match = /^F ([0-9]{1,3}) ([0-9]{1,3}) ([A-Za-z0-9+/=]+)$/.exec(line);
  if (!match) return;
  const columns = Number(match[1]);
  const rows = Number(match[2]);
  if (columns < 1 || columns > MAX_COLUMNS || rows < 1 || rows > MAX_ROWS) return;
  const cells = Buffer.from(match[3], "base64");
  // Re-encoding catches base64 that Buffer decoded leniently.
  if (cells.length !== columns * rows * CELL_BYTES || cells.toString("base64") !== match[3]) return;
  for (let i = 0; i < cells.length; i += CELL_BYTES) {
    const rune = cells.readUInt32LE(i);
    // The producer guarantees one BMP cell. Reject terminal controls anyway,
    // so a bad binary cannot inject escapes into the host.
    if (rune < 32 || rune > 0xffff) return;
    if (rune >= 0x7f && rune <= 0x9f) return;
    if (rune >= 0xd800 && rune <= 0xdfff) return;
    if (cells.readUInt32LE(i + 4) > DEFAULT || cells.readUInt32LE(i + 8) > DEFAULT) return;
  }
  return { columns, rows, cells };
}

// color is the SGR sequence for one 24-bit colour, or for the terminal's default.
function color(value: number, background: boolean): string {
  if (value === DEFAULT) return `\x1b[${background ? 49 : 39}m`;
  return `\x1b[${background ? 48 : 38};2;${value >> 16};${(value >> 8) & 255};${value & 255}m`;
}

/**
 * Turns a frame into one string per row for a host that owns the terminal.
 * Only SGR colour sequences and printable cells go back.
 */
export function frameLines(frame: Frame): string[] {
  const lines: string[] = [];
  for (let y = 0; y < frame.rows; y++) {
    let line = "";
    let fg = -1;
    let bg = -1;
    for (let x = 0; x < frame.columns; x++) {
      const at = (y * frame.columns + x) * CELL_BYTES;
      const nextFG = frame.cells.readUInt32LE(at + 4);
      const nextBG = frame.cells.readUInt32LE(at + 8);
      if (nextFG !== fg) {
        line += color(nextFG, false);
        fg = nextFG;
      }
      if (nextBG !== bg) {
        line += color(nextBG, true);
        bg = nextBG;
      }
      line += String.fromCodePoint(frame.cells.readUInt32LE(at));
    }
    lines.push(line + "\x1b[0m");
  }
  return lines;
}

/**
 * Maps a host's key name to the game's Bubble Tea name, or undefined to drop it.
 * Anything returned is safe to write as one line of the input file.
 */
export function keyName(key: string): string | undefined {
  const names: Record<string, string> = {
    escape: "esc",
    return: "enter",
    pageUp: "pgup",
    pageDown: "pgdown",
    pageup: "pgup",
    pagedown: "pgdown",
    " ": "space",
    "ctrl+[": "esc",
  };
  const name = names[key] || key;
  if (name.length === 1 && name >= " " && name !== "\x7f") return name;
  if (KNOWN_KEY.test(name)) return name;
}

/**
 * The environment bin/lumon-hook runs with, for an inline game or a status event.
 * It always turns off the native opening delay, which would open a window.
 */
export function inlineEnv(
  data: string,
  mode: Mode,
  options: Record<string, unknown> = {},
): NodeJS.ProcessEnv {
  const company = String(options.company ?? process.env.LUMON_COMPANY ?? "0");
  return {
    ...process.env,
    LUMON_PLUGIN_DATA: data,
    CLAUDE_PLUGIN_DATA: "",
    LUMON_DELAY: "off",
    CLAUDE_PLUGIN_OPTION_DELAY: "off",
    LUMON_COMPANY: company,
    CLAUDE_PLUGIN_OPTION_COMPANY: company,
    LUMON_MODE: mode,
    CLAUDE_PLUGIN_OPTION_MODE: mode,
  };
}

// ---- Status and automatic opening ----

/** Sends one agent status event through bin/lumon-hook in the background, ignoring any failure. */
export function sendStatus(
  root: string,
  data: string,
  event: string,
  session?: string,
  cwd?: string,
  tool?: Tool,
): void {
  if (!STATUS_EVENTS.includes(event)) return;
  if (session !== undefined && !/^[A-Za-z0-9_-]{1,128}$/.test(session)) return;
  try {
    const child = spawn("sh", [join(root, "bin/lumon-hook"), event], {
      detached: true,
      stdio: ["pipe", "ignore", "ignore"],
      env: inlineEnv(data, "play"),
    });
    child.on("error", () => {});
    child.stdin.on("error", () => {});
    child.stdin.end(session === undefined ? "" : JSON.stringify({ session_id: session, cwd, ...tool }));
    child.unref();
  } catch {
    /* Status must not interrupt the agent. */
  }
}

/**
 * Reports whether the agent integration is switched off, for good or until a time.
 * An unreadable switch counts as off.
 */
export function agentsOff(now = Date.now()): boolean {
  if (process.env.LUMON_AGENTS === "off") return true;
  try {
    // The file holds a Unix time in seconds to snooze until; anything else is off.
    const text = readFileSync(join(statusDir(), "off"), "utf8").trim();
    return !/^[0-9]+$/.test(text) || Number(text) * 1000 > now;
  } catch (error) {
    return (error as NodeJS.ErrnoException).code !== "ENOENT";
  }
}

// claimAutomatic respects the native launch guard too: another agent may
// already have a game, or be starting one. Any doubt means no.
function claimAutomatic(): boolean {
  const status = statusDir();
  try {
    const pid = Number(readFileSync(join(status, "game.pid"), "utf8").trim());
    if (Number.isSafeInteger(pid) && pid > 0) {
      try {
        process.kill(pid, 0);
        return false;
      } catch (error) {
        if ((error as NodeJS.ErrnoException).code !== "ESRCH") return false;
      }
    }
  } catch (error) {
    if ((error as NodeJS.ErrnoException).code !== "ENOENT") return false;
  }
  try {
    mkdirSync(status, { recursive: true, mode: 0o700 });
    const lock = join(status, "launch.lock");
    try {
      if (Date.now() - statSync(lock).mtimeMs < LAUNCH_LOCK_MS) return false;
      rmSync(lock);
    } catch (error) {
      if ((error as NodeJS.ErrnoException).code !== "ENOENT") return false;
    }
    writeFileSync(lock, "", { flag: "wx", mode: 0o600 });
    return true;
  } catch {
    return false;
  }
}

/**
 * One cancellable automatic opening per interactive host, for a prompt that
 * runs long. The off switch is checked again when the timer fires.
 */
export class AutoOpen {
  private timer?: ReturnType<typeof setTimeout>;

  /** Schedules `open` after `delay` seconds, replacing any earlier one; "off" or 0 never opens. */
  prompt(delay: unknown, open: () => void): void {
    this.cancel();
    const value = String(delay ?? "off");
    if (!/^[0-9]+$/.test(value) || Number(value) <= 0 || agentsOff()) return;
    const fire = () => {
      this.timer = undefined;
      if (!agentsOff() && claimAutomatic()) open();
    };
    this.timer = setTimeout(fire, Math.min(MAX_DELAY_SECONDS, Number(value)) * 1000);
    this.timer.unref?.();
  }

  /** Drops a scheduled opening, if any. */
  cancel(): void {
    if (this.timer) clearTimeout(this.timer);
    this.timer = undefined;
  }
}

// ---- The inline game ----

/**
 * One game process drawn inside the host. Frames arrive on stdout; keys and
 * sizes go back through an input file the game reads each frame.
 */
export class InlineGame {
  private child?: ChildProcess;
  private dir?: string;
  private sequence = 0;
  private events: string[] = [];
  private columns = 0;
  private rows = 0;
  private stopped = false;
  private shown = false;
  private pending = "";
  private errors = "";
  private timeout?: ReturnType<typeof setTimeout>;
  private killTimeout?: ReturnType<typeof setTimeout>;

  constructor(
    private root: string,
    private data: string,
    private changed: (frame: Frame) => void,
    private exited: (message?: string) => void,
  ) {}

  /** Starts the game once; a stopped game cannot be started again. */
  start(mode: Mode, options: Record<string, unknown> = {}): void {
    if (this.child || this.stopped) return;
    try {
      if (process.platform === "win32") throw new Error("Lumon inline requires a POSIX host.");
      mkdirSync(this.data, { recursive: true, mode: 0o700 });
      this.dir = mkdtempSync(join(this.data, "inline-"));
      this.write();
      const child = spawn("sh", [join(this.root, "bin/lumon-hook"), "pane"], {
        detached: true,
        stdio: ["ignore", "pipe", "pipe"],
        env: { ...inlineEnv(this.data, mode, options), LUMON_PANE_INPUT: join(this.dir, "input") },
      });
      this.child = child;
      child.on("error", error => this.finish(error.message));
      child.on("close", () => {
        if (this.killTimeout) clearTimeout(this.killTimeout);
        this.finish(this.shown ? undefined : this.errors.trim() || "Lumon exited before drawing a frame.");
      });
      child.stdout.on("data", chunk => this.output(String(chunk)));
      child.stderr.on("data", chunk => {
        this.errors = (this.errors + String(chunk)).slice(0, MAX_ERROR_OUTPUT);
      });
      const late = () => this.finish("Lumon did not start within 130 seconds.");
      this.timeout = setTimeout(late, START_TIMEOUT_MS);
      this.timeout.unref?.();
    } catch (error) {
      this.finish(String(error));
    }
  }

  /** Asks the game to draw at this size, clamped to what it supports. */
  resize(columns: number, rows: number): void {
    columns = Math.max(1, Math.min(MAX_COLUMNS, Math.floor(columns)));
    rows = Math.max(1, Math.min(MAX_ROWS, Math.floor(rows)));
    if (!Number.isFinite(columns) || !Number.isFinite(rows)) return;
    if (columns === this.columns && rows === this.rows) return;
    this.columns = columns;
    this.rows = rows;
    this.event(`size ${columns} ${rows}`);
  }

  /** Passes one key to the game; names the game does not know are dropped. */
  key(name: string): void {
    const key = keyName(name);
    if (key) this.event(`key ${key}`);
  }

  /** Stops the game and removes its input folder. */
  stop(): void {
    this.finish();
  }

  // output splits stdout into frame lines; one bad line ends the game.
  private output(chunk: string): void {
    if (this.stopped) return;
    this.pending += chunk;
    let nl: number;
    while ((nl = this.pending.indexOf("\n")) >= 0) {
      if (nl > MAX_LINE) {
        this.finish("Lumon sent an oversized frame.");
        return;
      }
      const line = this.pending.slice(0, nl);
      this.pending = this.pending.slice(nl + 1);
      const frame = parseFrame(line);
      if (!frame) {
        this.finish("Lumon sent an invalid frame.");
        return;
      }
      this.shown = true;
      if (this.timeout) clearTimeout(this.timeout);
      this.changed(frame);
    }
    if (this.pending.length > MAX_LINE) this.finish("Lumon sent an oversized frame.");
  }

  // event numbers each line so the game can tell new events from ones it has read.
  private event(text: string): void {
    if (this.stopped) return;
    this.events = [...this.events, `${++this.sequence} ${text}`].slice(-MAX_EVENTS);
    try {
      this.write();
    } catch (error) {
      this.finish(String(error));
    }
  }

  // write replaces the input file by rename, so the game never reads half of it.
  private write(): void {
    if (!this.dir) return;
    const path = join(this.dir, "input");
    writeFileSync(path + ".tmp", this.events.join("\n") + "\n", { mode: 0o600 });
    renameSync(path + ".tmp", path);
  }

  private finish(message?: string): void {
    if (this.stopped) return;
    this.stopped = true;
    if (this.timeout) clearTimeout(this.timeout);
    const child = this.child;
    if (child?.pid && child.exitCode === null && child.signalCode === null) {
      try {
        process.kill(-child.pid, "SIGTERM");
      } catch {
        /* Already stopped. */
      }
      // Reap a game that ignores TERM, including the shell's descendants.
      const kill = () => {
        try {
          process.kill(-child.pid!, "SIGKILL");
        } catch {}
      };
      this.killTimeout = setTimeout(kill, KILL_GRACE_MS);
      this.killTimeout.unref();
    }
    if (this.dir) rmSync(this.dir, { recursive: true, force: true });
    // The message may come from the game's stderr; strip controls before the host shows it.
    queueMicrotask(() => this.exited(message?.replace(/[\x00-\x1f\x7f-\x9f]/g, " ").slice(0, MAX_MESSAGE)));
  }
}
