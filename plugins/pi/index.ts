// Lumon inside Pi: slash commands draw the game in Pi's own terminal UI, and
// agent events go to the shared status folder through bin/lumon-hook.
import { homedir } from "node:os";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import type { ExtensionAPI, ExtensionContext } from "@earendil-works/pi-coding-agent";
import { parseKey, truncateToWidth } from "@earendil-works/pi-tui";
import { AutoOpen, InlineGame, frameLines, modeOf, sendStatus, type Frame, type Mode } from "./inline.js";

// ---- Constants ----

const root = dirname(fileURLToPath(import.meta.url));
const dataDir = () => join(process.env.PI_CODING_AGENT_DIR || join(homedir(), ".pi/agent"), "lumon");

// The game's own size limits, as in inline.ts.
const MAX_COLUMNS = 512;
const MAX_ROWS = 256;

// Rows of Pi's terminal not given to the game: the title line and Pi's own.
const RESERVED_ROWS = 3;

type Tool = { tool_name: string; tool_input: unknown };

// The commands that open the game, one per mode.
const OPENERS = [
  ["lumon-play", "play", "Play Lumon inside Pi"],
  ["lumon-watch", "watch", "Watch computer refiners inside Pi"],
  ["lumon-screensaver", "screensaver", "Show Lumon's screensaver inside Pi"],
] as const;

// The commands that switch the agent integration, named after the event they send.
const SWITCHES = ["off", "snooze", "on"] as const;

// ---- The plugin ----

/** Registers Lumon's commands and agent event handlers with Pi. */
export default function lumon(pi: ExtensionAPI) {
  const automatic = new AutoOpen();
  let active: InlineGame | undefined;
  let pending = false;
  const stopActive = () => active?.stop();
  const send = (event: string, ctx?: ExtensionContext, tool?: Tool) =>
    sendStatus(root, dataDir(), event, ctx?.sessionManager.getSessionId(), ctx?.cwd, tool);

  // open shows the game as a custom component until the game ends or the player leaves.
  async function open(ctx: ExtensionContext, mode: Mode): Promise<void> {
    if (ctx.mode !== "tui") {
      ctx.ui.notify("Lumon requires Pi's interactive terminal UI.", "error");
      return;
    }
    if (pending || active) return;
    pending = true;
    automatic.cancel();
    try {
      await ctx.ui.custom<void>((tui, _theme, _keys, done) => {
        let frame: Frame | undefined;
        let lines: string[] = [];
        let disposed = false;
        const changed = (next: Frame) => {
          frame = next;
          lines = frameLines(next);
          tui.requestRender();
        };
        const exited = (message?: string) => {
          if (disposed) return;
          disposed = true;
          if (message) ctx.ui.notify(message, "error");
          done();
        };
        const game = new InlineGame(root, dataDir(), changed, exited);
        active = game;
        game.start(mode);
        return {
          render(width: number): string[] {
            const columns = Math.max(1, Math.min(MAX_COLUMNS, width));
            const rows = Math.max(1, Math.min(MAX_ROWS, tui.terminal.rows - RESERVED_ROWS));
            game.resize(columns, rows);
            // Until a frame of the new size arrives, an old one would wrap or be cut.
            const body = frame?.columns === columns && frame.rows === rows ? lines : ["Loading Lumon…"];
            return ["LUMON · Ctrl+] returns to Pi", ...body].map(line => truncateToWidth(line, width));
          },
          handleInput(input: string): void {
            const parsed = parseKey(input);
            if (parsed === "ctrl+]" || parsed === "ctrl+c") {
              game.stop();
              return;
            }
            // Printable characters go as typed; parseKey names the rest.
            const key = input.length === 1 && input >= " " ? input : parsed;
            if (key) game.key(key);
          },
          invalidate(): void {},
          dispose(): void {
            disposed = true;
            game.stop();
          },
        };
      });
    } catch (error) {
      ctx.ui.notify(`Lumon could not open inline: ${String(error)}`, "error");
    } finally {
      stopActive();
      active = undefined;
      pending = false;
    }
  }

  // Agent events. Only the interactive UI reports status or opens the game.
  pi.on("session_start", (_event, ctx) => {
    if (ctx.mode === "tui") send("start", ctx);
  });
  pi.on("input", (event, ctx) => {
    if (event.source !== "interactive" || ctx.mode !== "tui") return;
    send("prompt", ctx);
    automatic.prompt(process.env.LUMON_DELAY, () => void open(ctx, modeOf(process.env.LUMON_MODE)));
  });
  pi.on("tool_execution_start", (event, ctx) => {
    // A tool run inside another tool call is part of that call's step.
    if (ctx.mode === "tui" && !event.parentToolCallId) {
      send("tool", ctx, { tool_name: event.toolName, tool_input: event.args });
    }
  });
  pi.on("ui_prompt_start", (_event, ctx) => {
    automatic.cancel();
    if (ctx.mode === "tui") send("waiting", ctx);
  });
  pi.on("agent_settled", (_event, ctx) => {
    automatic.cancel();
    if (ctx.mode === "tui") send("stop", ctx);
  });
  pi.on("session_shutdown", (_event, ctx) => {
    automatic.cancel();
    active?.stop();
    if (ctx.mode === "tui") send("end", ctx);
  });
  pi.on("session_before_switch", () => {
    automatic.cancel();
    active?.stop();
  });

  // Commands.
  for (const [name, mode, description] of OPENERS) {
    pi.registerCommand(name, { description, handler: async (_args, ctx) => open(ctx, mode) });
  }
  for (const event of SWITCHES) {
    const verb = event === "on" ? "Enable" : "Pause";
    const until = event === "snooze" ? " until midnight" : "";
    const state = event === "off" ? "off" : "off until midnight";
    const notice =
      event === "on"
        ? "Lumon agent integration is enabled; automatic opening still requires a delay."
        : `Lumon agent integration is ${state}. Manual play still works.`;
    pi.registerCommand(`lumon-${event}`, {
      description: `${verb} Lumon agent integration${until}`,
      handler: async (_args, ctx) => {
        automatic.cancel();
        send(event);
        ctx.ui.notify(notice, "info");
      },
    });
  }
}
