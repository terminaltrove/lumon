// Server-side status only. Gameplay and manual commands live in the TUI entry
// point, so they cost no model turn and cannot open an external terminal.
import { homedir } from "node:os";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import type { Plugin as PluginV1 } from "@opencode-ai/plugin";
import type { Plugin as PluginV2 } from "@opencode/plugin";
import { sendStatus } from "./inline.js";

// ---- Constants ----

const root = dirname(fileURLToPath(import.meta.url));
const data = join(process.env.XDG_DATA_HOME || join(homedir(), ".local/share"), "opencode/lumon");

type Tool = { tool_name: string; tool_input: unknown };

// Tool names remembered between a call's input and its execution; past this the map is
// cleared, so calls that never execute cannot grow it.
const MAX_PENDING_TOOLS = 256;

// OpenCode 2's events and the status event each sends. Tool events are handled apart.
const STATUS_OF: Record<string, string> = {
  "session.created": "start",
  "session.execution.started": "prompt",
  "permission.asked": "waiting",
  "session.execution.succeeded": "stop",
  "session.execution.failed": "idle",
  "session.execution.interrupted": "idle",
  "session.deleted": "end",
};

// Every OpenCode 2 event the plugin listens for.
const V2_EVENTS = [
  "session.created",
  "session.execution.started",
  "session.tool.input.started",
  "session.tool.called",
  "permission.asked",
  "session.execution.succeeded",
  "session.execution.failed",
  "session.execution.interrupted",
  "session.deleted",
] as const;

// ---- OpenCode 1 ----

// server reports status for top-level sessions; a subagent's child session
// would otherwise look like a second agent.
const server: PluginV1 = async input => {
  const children = new Set<string>();
  const send = (event: string, session?: string, cwd = input.directory, tool?: Tool) =>
    sendStatus(root, data, event, session, cwd, tool);
  return {
    "chat.message": async ({ sessionID }) => {
      if (!children.has(sessionID)) send("prompt", sessionID);
    },
    "tool.execute.before": async ({ tool, sessionID }, { args }) => {
      if (children.has(sessionID)) return;
      send("tool", sessionID, input.directory, { tool_name: tool, tool_input: args });
    },
    event: async ({ event }) => {
      const props = event.properties as {
        sessionID?: string;
        info?: { id: string; directory: string; parentID?: string };
      };
      switch (event.type as string) {
        case "session.created":
          if (props.info?.parentID) children.add(props.info.id);
          else if (props.info) send("start", props.info.id, props.info.directory);
          break;
        case "permission.updated":
        case "permission.asked":
          if (props.sessionID && !children.has(props.sessionID)) send("waiting", props.sessionID);
          break;
        case "session.idle":
          if (props.sessionID && !children.has(props.sessionID)) send("stop", props.sessionID);
          break;
        case "session.deleted":
          if (props.info && !children.delete(props.info.id)) send("end", props.info.id, props.info.directory);
          break;
      }
    },
  };
};

// ---- OpenCode 2 ----

// setup reports status from OpenCode 2's event stream until the plugin is disposed.
const setup: PluginV2.Plugin["setup"] = async ctx => {
  const stop = new AbortController();
  // A tool's name arrives with its input; its arguments arrive when it is called.
  const names = new Map<string, string>();
  const events = ctx.event.subscribe({ types: V2_EVENTS, signal: stop.signal });
  void (async () => {
    for await (const event of events) {
      const session = event.data.sessionID;
      switch (event.type) {
        case "session.tool.input.started":
          if (names.size > MAX_PENDING_TOOLS) names.clear();
          names.set(event.data.id, event.data.name);
          break;
        case "session.tool.called": {
          const name = names.get(event.data.id);
          names.delete(event.data.id);
          if (name) {
            const tool = { tool_name: name, tool_input: event.data.input };
            sendStatus(root, data, "tool", session, ctx.location.directory, tool);
          }
          break;
        }
        default: {
          const state = STATUS_OF[event.type];
          if (state) sendStatus(root, data, state, session, ctx.location.directory);
        }
      }
    }
  })().catch(() => {});
  return () => stop.abort();
};

/** Lumon's server plugin, for both OpenCode 1 (`server`) and OpenCode 2 (`setup`). */
export default { id: "lumon", server, setup };
