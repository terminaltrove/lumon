// Tests for the inline transport shared by the Pi and OpenCode plugins, and for
// each plugin's entry point. Tests that need the native game binary run only
// when LUMON_TEST_BINARY names one (scripts/test-inline.sh builds it).
import { describe, test, expect, afterEach } from "bun:test";
import { mkdtempSync, rmSync, readFileSync, writeFileSync, readdirSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { createRoot } from "solid-js";
import { createTestRenderer } from "@opentui/core/testing";
import { InlineGame, parseFrame, frameLines, keyName, AutoOpen, inlineEnv } from "./pi/inline.js";
import piPlugin from "./pi/index.js";
import opencode from "./opencode/tui.js";

// The host APIs below are mocked with only the parts each plugin touches, so
// the mocks are typed loosely (`any`) instead of against the full host types.

// ---- Helpers ----

// The native game, when the test script built one.
const binary = process.env.LUMON_TEST_BINARY;

// The automatic opening delay used below is "1" second; waits a little longer.
const PAST_ONE_SECOND_MS = 1050;

const saved = { ...process.env };
const temporary: string[] = [];
const cleanup: (() => void)[] = [];

// Each test gets back the environment it started with, and its folders and
// games are gone before the next one runs.
afterEach(() => {
  for (const dispose of cleanup.splice(0).reverse()) dispose();
  process.env = { ...saved };
  for (const dir of temporary.splice(0)) rmSync(dir, { recursive: true, force: true });
});

// temp makes a folder that is removed after the test.
function temp(): string {
  const dir = mkdtempSync(join(tmpdir(), "lumon-inline-test-"));
  temporary.push(dir);
  return dir;
}

// wait sleeps for a short time, in ms.
const wait = (ms = 20) => new Promise(resolve => setTimeout(resolve, ms));

// until polls check until it holds, or fails after timeout ms.
async function until(check: () => boolean, timeout = 5000): Promise<void> {
  const deadline = Date.now() + timeout;
  while (!check()) {
    if (Date.now() > deadline) throw new Error("Timed out waiting for inline UI");
    await wait();
  }
}

// environment points every folder the plugins and the shim use at fresh
// temporary ones, and the shim at the test binary when there is one.
function environment(): void {
  process.env.XDG_DATA_HOME = temp();
  process.env.PI_CODING_AGENT_DIR = temp();
  process.env.LUMON_STATUS_DIR = temp();
  process.env.LUMON_PLUGIN_DATA = temp();
  if (process.env.LUMON_TEST_BINARY) process.env.LUMON_BIN = process.env.LUMON_TEST_BINARY;
}

// cell writes one 12-byte frame cell: code point, foreground, background.
function cell(cells: Buffer, rune: number, fg: number, bg: number): void {
  cells.writeUInt32LE(rune, 0);
  cells.writeUInt32LE(fg, 4);
  cells.writeUInt32LE(bg, 8);
}

// ---- Tests ----

describe("frame transport", () => {
  // Pi and OpenCode install separately, so each carries a copy; they must not drift.
  test("independent installs carry identical frame transports", () => {
    const piCopy = readFileSync(new URL("./pi/inline.ts", import.meta.url), "utf8");
    const opencodeCopy = readFileSync(new URL("./opencode/inline.ts", import.meta.url), "utf8");
    expect(piCopy).toBe(opencodeCopy);
  });

  // A frame's colours survive as SGR, and nothing a bad binary sends can reach
  // the host terminal as a control sequence or an unknown key.
  test("frames preserve RGB cells and reject terminal control injection", () => {
    const cells = Buffer.alloc(12);
    cell(cells, 65, 0x123456, 0x1000000);
    const encoded = () => `F 1 1 ${cells.toString("base64")}`;

    const frame = parseFrame(encoded())!;
    expect(frameLines(frame)).toEqual(["\x1b[38;2;18;52;86m\x1b[49mA\x1b[0m"]);

    // ESC as a cell, a size past the limit, and cells that do not fill the size.
    cells.writeUInt32LE(27, 0);
    expect(parseFrame(encoded())).toBeUndefined();
    expect(parseFrame("F 999 1 AAAA")).toBeUndefined();
    expect(parseFrame("F 1 1 AAAA")).toBeUndefined();

    // A key name with a newline would write a second event into the input file.
    expect(keyName("x\n2 quit")).toBeUndefined();
    expect(keyName("escape")).toBe("esc");
    expect(keyName("shift+left")).toBe("shift+left");
  });
});

describe("environment and activation", () => {
  // The native delay opens a separate window, which an inline host must never do.
  test("status environment never enables a native window-opening delay", () => {
    process.env.CLAUDE_PLUGIN_OPTION_DELAY = "30";
    const env = inlineEnv(temp(), "watch");
    expect(env.LUMON_DELAY).toBe("off");
    expect(env.CLAUDE_PLUGIN_OPTION_DELAY).toBe("off");
    expect(env.CLAUDE_PLUGIN_OPTION_MODE).toBe("watch");
  });

  // Automatic opening needs a delay, stops on cancel, the off switch or a
  // running game, and opens once for one prompt.
  test("automatic activation is opt-in, cancellable and respects off", async () => {
    environment();
    const status = process.env.LUMON_STATUS_DIR!;
    const automatic = new AutoOpen();
    cleanup.push(() => automatic.cancel());
    let opened = 0;
    const open = () => opened++;

    // No delay set: never opens.
    automatic.prompt(undefined, open);
    await wait(30);
    expect(opened).toBe(0);

    // Cancelled before it fires.
    automatic.prompt("1", open);
    automatic.cancel();
    await wait(PAST_ONE_SECOND_MS);
    expect(opened).toBe(0);

    // Switched off after it was scheduled.
    automatic.prompt("1", open);
    writeFileSync(join(status, "off"), "");
    await wait(PAST_ONE_SECOND_MS);
    expect(opened).toBe(0);
    rmSync(join(status, "off"));

    // Another game is already running (this test process stands in for it).
    writeFileSync(join(status, "game.pid"), String(process.pid));
    automatic.prompt("1", open);
    await wait(PAST_ONE_SECOND_MS);
    expect(opened).toBe(0);
    rmSync(join(status, "game.pid"));

    // Free to open: opens once.
    automatic.prompt("1", open);
    await until(() => opened === 1, 1500);

    // A second prompt inside the launch lock's hold does not open another.
    automatic.prompt("1", open);
    await wait(PAST_ONE_SECOND_MS);
    expect(opened).toBe(1);
  }, 10000);
});

describe("inline game process", () => {
  // A game that cannot start says why, and does not fall back to a window.
  test("a failed inline startup reports the error without a window fallback", async () => {
    environment();
    process.env.LUMON_BIN = join(temp(), "missing");
    let error = "";
    const exited = (message?: string) => {
      error = message ?? "";
    };
    const game = new InlineGame(new URL("./pi/", import.meta.url).pathname, temp(), () => {}, exited);
    cleanup.push(() => game.stop());
    game.start("play");
    await until(() => !!error);
    expect(error).toContain("LUMON_BIN");
  });

  // The game draws at the size asked, follows a resize, reads keys from its
  // input file, and leaves nothing behind when stopped.
  test.skipIf(!binary)("real game frames, resize, inputs and process cleanup", async () => {
    environment();
    const data = temp();
    let frame: { columns: number; rows: number } | undefined;
    const exited = (message?: string) => {
      if (message) throw new Error(message);
    };
    const changed = (next: { columns: number; rows: number }) => {
      frame = next;
    };
    const game = new InlineGame(new URL("./pi/", import.meta.url).pathname, data, changed, exited);
    cleanup.push(() => game.stop());

    game.start("watch", { company: "1" });
    game.resize(100, 30);
    await until(() => frame?.columns === 100 && frame.rows === 30);

    game.key("left");
    const input = join(data, readdirSync(data)[0], "input");
    expect(readFileSync(input, "utf8")).toContain("key left");

    game.resize(80, 24);
    await until(() => frame?.columns === 80 && frame.rows === 24);

    game.stop();
    expect(readdirSync(data)).toHaveLength(0);
  });
});

describe("Pi integration", () => {
  // /lumon-play draws the game in Pi's UI, passes keys, and Ctrl+] gives Pi
  // its terminal back. Outside the TUI it draws nothing.
  test.skipIf(!binary)("Pi consumes slash commands, draws the real game and returns focus", async () => {
    environment();
    const commands = new Map<string, any>();
    const handlers = new Map<string, any>();
    const pi = {
      registerCommand: (name: string, command: unknown) => commands.set(name, command),
      on: (name: string, fn: unknown) => handlers.set(name, fn),
    };
    piPlugin(pi as any);
    expect(commands.size).toBe(6);

    // ui.custom hands the component to the test and resolves when it is done.
    let component: any;
    let finish: () => void = () => {};
    let changed = 0;
    const tui = { terminal: { rows: 32 }, requestRender: () => changed++ };
    const ctx = {
      mode: "tui",
      cwd: temp(),
      sessionManager: { getSessionId: () => "session-test" },
      ui: {
        notify: (message: string, type: string) => {
          if (type === "error") throw new Error(message);
        },
        custom: (factory: any) =>
          new Promise<void>(resolve => {
            finish = resolve;
            component = factory(tui, {}, {}, resolve);
          }),
      },
    };

    const running = commands.get("lumon-play").handler("", ctx);
    await until(() => !!component);
    component.render(100);
    await until(() => changed > 0);
    // 32 terminal rows less Pi's reserve, plus the title line.
    expect(component.render(100).length).toBe(30);

    component.handleInput("\x1b[C"); // right arrow
    component.handleInput("\x1d"); // Ctrl+]
    await running;
    component.dispose();
    finish();

    // Non-interactive invocations must not construct a terminal component.
    let used = false;
    const rpc = {
      ...ctx,
      mode: "rpc",
      ui: {
        notify() {},
        custom() {
          used = true;
        },
      },
    };
    await commands.get("lumon-play").handler("", rpc);
    expect(used).toBe(false);
  });
});

describe("OpenCode integration", () => {
  // OpenCode 2: /lumon-play opens the game page in OpenTUI, draws the game's cells,
  // and Ctrl+] returns to the session and releases the key mode.
  test.skipIf(!binary)(
    "OpenCode 2 slash command renders real cells in native OpenTUI and returns to chat",
    async () => {
      environment();
      const terminal = await createTestRenderer({ width: 100, height: 32 });
      cleanup.push(() => terminal.renderer.destroy());

      let commands: any[] = [];
      let page: any;
      let route: any = { type: "session", sessionID: "session-test" };
      let modeReleased = false;
      let pageDispose = () => {};
      let pluginDispose = () => {};
      let view: any;

      // Navigating to the plugin's page renders it into the test terminal;
      // navigating anywhere else disposes it.
      const navigate = (destination: any) => {
        route = destination;
        if (destination.type === "plugin") {
          createRoot(dispose => {
            pageDispose = dispose;
            view = page.render({});
            terminal.renderer.root.add(view);
            view.focus();
          });
        } else {
          pageDispose();
        }
      };
      const ctx = {
        renderer: terminal.renderer,
        options: {},
        data: { listen: () => () => {} },
        keymap: {
          layer: (factory: any) => {
            commands = factory().commands;
          },
          mode: {
            push: () => () => {
              modeReleased = true;
            },
          },
        },
        ui: {
          toast: {
            show: (message: any) => {
              throw new Error(message.message);
            },
          },
          router: {
            current: () => route,
            register: (value: any) => {
              page = value;
              return () => {};
            },
            navigate,
          },
        },
      };

      createRoot(dispose => {
        pluginDispose = dispose;
        const stop = opencode.setup(ctx as any);
        cleanup.push(() => {
          if (typeof stop === "function") void stop();
        });
      });
      cleanup.push(() => pluginDispose());

      const play = commands.find(command => command.slash.name === "lumon-play");
      expect(play).toBeDefined();
      play.run();
      await terminal.renderOnce();

      // Render until the game's own frame replaces the loading line; on timeout
      // the error shows what the terminal held.
      const drawn = () => {
        const screen = terminal.captureCharFrame();
        return !screen.includes("Loading Lumon") && screen.includes("LUMON");
      };
      const limit = Date.now() + 5000;
      while (!drawn()) {
        if (Date.now() > limit) throw new Error(terminal.captureCharFrame());
        await wait();
        await terminal.renderOnce();
      }
      expect(terminal.captureCharFrame()).toContain("Ctrl+] returns to chat");

      terminal.mockInput.pressKey("]", { ctrl: true });
      await until(() => route.type === "session");
      expect(modeReleased).toBe(true);
    },
    10000,
  );

  // OpenCode 1: the TUI plugin registers its page and the six local slash
  // commands, none of which goes through the model.
  test("OpenCode 1 registers local slash commands and no model commands", async () => {
    let commands: any[] = [];
    let page: any;
    const disposers: (() => void)[] = [];
    const api = {
      renderer: {},
      command: {
        register: (factory: any) => {
          commands = factory();
          return () => {};
        },
      },
      route: {
        register: (pages: any[]) => {
          page = pages[0];
          return () => {};
        },
        current: { name: "home" },
      },
      event: { on: () => () => {} },
      lifecycle: { onDispose: (dispose: () => void) => disposers.push(dispose) },
      ui: { toast() {} },
    };

    await opencode.tui(api as any);
    expect(page.name).toBe("lumon");
    expect(commands.map(command => command.slash.name)).toEqual([
      "lumon-play",
      "lumon-watch",
      "lumon-screensaver",
      "lumon-off",
      "lumon-snooze",
      "lumon-on",
    ]);
    for (const dispose of disposers) dispose();
  });
});
