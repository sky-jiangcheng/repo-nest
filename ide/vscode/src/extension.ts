// Repotory for VS Code — thin client over Repotory (ADR-0009 step two / TODO M5).
//
// Commands: load session context, write the session handoff and search the
// knowledge base; a status-bar item and a knowledge-search sidebar make the
// product visible inside the IDE. Everything is delegated: read endpoints go
// to the loopback-only headless service when it is up, all tools go through
// repotory-mcp over stdio — zero logic duplication (thin-client discipline).
import * as vscode from "vscode";
import * as path from "node:path";
import { accessSync } from "node:fs";
import { McpStdioClient } from "./mcpClient.js";
import { getJson, postJson } from "./headless.js";
import { NotesSearchProvider, parseSearchHits } from "./notesTree.js";

const CONFIG_SECTION = "repotory";
const SERVER_URL_KEY = "serverUrl";
const PROJECT_ID_KEY = "projectId";
const BINARY_NAMES = ["repotory-mcp", "repotory-mcp.exe"];

export let output: vscode.OutputChannel;
let statusItem: vscode.StatusBarItem;
let mcp: McpStdioClient | null = null;

interface HandoffStatus {
  note_id?: number;
  project_id?: number;
  title?: string;
  updated_at?: string;
}

function serverUrl(): string {
  return (
    vscode.workspace.getConfiguration(CONFIG_SECTION).get<string>(SERVER_URL_KEY) ??
    "http://127.0.0.1:18765"
  );
}

// workspacePath is the folder the session is actually standing in. It is the
// input the backend resolves into a project AND a repository, which is what makes
// opening `…/CBiPay/pay-common` behave differently from opening `…/CBiPay`.
function workspacePath(): string | undefined {
  const folder = vscode.workspace.workspaceFolders?.[0];
  return folder ? folder.uri.fsPath : undefined;
}

interface WorkspaceResolution {
  Project?: { id: number; name: string; root_path: string } | null;
  Repository?: { id: number; path: string } | null;
  MatchedOn?: string;
  Candidates?: unknown[];
}

let lastResolution: WorkspaceResolution | null = null;

function configuredProjectId(): number | undefined {
  const raw = vscode.workspace.getConfiguration(CONFIG_SECTION).get<string>(PROJECT_ID_KEY) ?? "";
  const id = Number(raw.trim());
  return Number.isInteger(id) && id > 0 ? id : undefined;
}

export function findBinary(): string | undefined {
  for (const name of BINARY_NAMES) {
    for (const dir of (process.env.PATH ?? "").split(path.delimiter)) {
      if (!dir) continue;
      const candidate = path.join(dir, name);
      try {
        accessSync(candidate);
        return candidate;
      } catch {
        /* keep looking */
      }
    }
  }
  return undefined;
}

async function getMcp(): Promise<McpStdioClient> {
  if (mcp) return mcp;
  const bin = findBinary();
  if (!bin) {
    const choice = await vscode.window.showErrorMessage(
      "Repotory: repotory-mcp binary not found on PATH. Install it from Releases, then retry.",
      "Open Releases",
    );
    if (choice === "Open Releases") {
      void vscode.env.openExternal(
        vscode.Uri.parse("https://github.com/sky-jiangcheng/repotory/releases"),
      );
    }
    throw new Error("repotory-mcp not found");
  }
  const client = new McpStdioClient({ command: bin }, (line: string) => output?.appendLine(line));
  await client.start();
  mcp = client;
  return client;
}

export async function callTool(name: string, args: Record<string, unknown>): Promise<string> {
  const client = await getMcp();
  try {
    return await client.callTool(name, args);
  } catch (err) {
    // A dead server must not wedge the extension: drop it so the next call restarts.
    mcp = null;
    client.stop();
    throw err;
  }
}

// refreshScope asks the backend which project/repository this workspace is, and
// shows the answer. The matching rules live server-side on purpose: the MCP tools
// resolve the same path, and a TypeScript reimplementation would let the sidebar
// and an agent's repotory_context disagree about which knowledge a session gets.
async function refreshScope(notes: NotesSearchProvider): Promise<void> {
  const dir = workspacePath();
  if (!dir) {
    lastResolution = null;
    notes.setScope("");
    return;
  }
  try {
    const response = (await postJson(serverUrl(), "/api/rpc", {
      method: "ResolveWorkspacePath",
      args: [dir],
    })) as { result?: WorkspaceResolution | null };
    const res = response.result ?? null;
    lastResolution = res;
    if (res?.Project) {
      const repo = res.Repository?.path ? res.Repository.path.split(/[\\/]/).pop() : undefined;
      notes.setScope(repo ? `${res.Project.name} · ${repo}` : res.Project.name);
      if (statusItem) {
        statusItem.tooltip =
          `Workspace: ${res.Project.name}${repo ? " / " + repo : ""}\n${dir}\n\nClick to load the session context`;
      }
    } else {
      notes.setScope("not in the knowledge base yet");
    }
  } catch {
    // No headless service: the commands still work through repotory-mcp, so the
    // sidebar stays silent about scope rather than asserting something unverified.
    notes.setScope("");
  }
}

async function pingHeadless(): Promise<boolean> {
  try {
    await getJson(serverUrl(), "/health");
    return true;
  } catch {
    return false;
  }
}

async function refreshLastHandoff(): Promise<void> {
  if (!statusItem) return;
  const projectId = configuredProjectId() ?? 0;
  try {
    const response = (await postJson(serverUrl(), "/api/rpc", {
      method: "LatestHandoff",
      args: [projectId],
    })) as { result?: HandoffStatus | null };
    const handoff = response.result;
    const at = handoff?.updated_at ? new Date(handoff.updated_at) : undefined;
    if (handoff && at && !Number.isNaN(at.getTime())) {
      statusItem.text = `$(nest) Repotory · ${at.toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" })}`;
      statusItem.tooltip = `Last handoff: ${handoff.title ?? "session"}\n${at.toLocaleString()}\n\nClick to load the session context`;
    } else {
      statusItem.text = "$(nest) Repotory";
      statusItem.tooltip = "Repotory: no session handoff yet. Click to load the session context";
    }
  } catch {
    statusItem.text = "$(nest) Repotory";
    statusItem.tooltip = "Repotory: click to load the session context";
  }
}

function withProgress<T>(title: string, task: () => Promise<T>): Thenable<T> {
  return vscode.window.withProgress(
    { location: vscode.ProgressLocation.Notification, title: `Repotory: ${title}` },
    task,
  );
}

function showOutput(title: string, body: string): void {
  output.appendLine(`\n=== ${title} ===\n${body}`);
  output.show(true);
}

// ---------------------------------------------------------------- commands

async function loadContext(): Promise<void> {
  const args: Record<string, unknown> = {};
  const projectId = configuredProjectId();
  if (projectId !== undefined) args.project_id = projectId;
  // The manual id wins when set; the path is what makes it work with no settings
  // at all.
  const dir = workspacePath();
  if (dir) args.path = dir;
  const text = await withProgress("loading session context", () =>
    callTool("repotory_context", args),
  );
  showOutput("Session context", text);
}

async function writeHandoff(): Promise<void> {
  // A configured id is an override; otherwise the backend resolves the project
  // from the workspace path. It used to be that this command simply refused
  // without a hand-typed number, which made the primary write path of the product
  // unusable out of the box.
  const configured = configuredProjectId();
  const dir = workspacePath();
  if (configured === undefined && !dir) {
    void vscode.window.showErrorMessage(
      "Repotory: open a folder, or set repotory.projectId — a handoff needs a project.",
    );
    return;
  }
  const summary = await vscode.window.showInputBox({
    prompt: "Repotory handoff — one paragraph: what was this session about?",
    placeHolder: "e.g. Wired the FTS5 rebuild path; tests green",
    ignoreFocusOut: true,
  });
  if (!summary) return;

  const askList = async (label: string): Promise<string[]> => {
    const raw = await vscode.window.showInputBox({
      prompt: `Repotory handoff — ${label} (comma-separated, empty to skip)`,
      ignoreFocusOut: true,
    });
    return raw
      ? raw
          .split(",")
          .map((s) => s.trim())
          .filter(Boolean)
      : [];
  };
  const changes = await askList("changes");
  const nextSteps = await askList("next steps");

  const handoffArgs: Record<string, unknown> = {
    agent: "vscode-extension",
    summary,
    changes,
    next_steps: nextSteps,
  };
  // project_id is sent only when the user pinned one; otherwise it is omitted and
  // the backend resolves it from `path`. Sending 0 as "unknown" would be a magic
  // number the tool has to special-case.
  if (configured !== undefined) handoffArgs.project_id = configured;
  if (dir) handoffArgs.path = dir;

  const noteText = await withProgress("writing session handoff", () =>
    callTool("repotory_handoff", handoffArgs),
  );
  showOutput("Handoff recorded", noteText);
  void refreshLastHandoff();
}

async function searchKnowledge(notes: NotesSearchProvider): Promise<void> {
  const query = await vscode.window.showInputBox({
    prompt: "Repotory — search notes & todos (FTS5)",
    placeHolder: "e.g. deploy hook",
    ignoreFocusOut: true,
  });
  if (!query) return;
  notes.setHint(`Searching "${query}"…`);
  try {
    const searchArgs: Record<string, unknown> = { query };
    // Ranking only: notes recorded in THIS repository lead the result list.
    const dir = workspacePath();
    if (dir) searchArgs.path = dir;
    const text = await withProgress("searching knowledge", () =>
      callTool("repotory_notes_search", searchArgs),
    );
    notes.setHits(parseSearchHits(text), query);
  } catch (err) {
    notes.setHint(`Search failed: ${err instanceof Error ? err.message : String(err)}`);
    throw err;
  }
}

async function registerMcp(): Promise<void> {
  const folder = vscode.workspace.workspaceFolders?.[0];
  if (!folder) {
    void vscode.window.showErrorMessage("Repotory: open a folder first.");
    return;
  }
  const script = vscode.Uri.joinPath(folder.uri, "scripts", "repotory-init", "index.mjs");
  try {
    await vscode.workspace.fs.stat(script);
  } catch {
    const choice = await vscode.window.showWarningMessage(
      "Repotory: scripts/repotory-init/index.mjs was not found in this workspace — the one-command installer lives in the Repotory repository.",
      "Open Docs",
    );
    if (choice === "Open Docs") {
      void vscode.env.openExternal(
        vscode.Uri.parse(
          "https://github.com/sky-jiangcheng/repotory/blob/master/docs/features/ai-integration.md",
        ),
      );
    }
    return;
  }
  const terminal = vscode.window.createTerminal({ name: "repotory-init", cwd: folder.uri });
  terminal.sendText("node scripts/repotory-init/index.mjs --with-hook");
  terminal.show();
}

// ---------------------------------------------------------------- lifecycle

export function activate(context: vscode.ExtensionContext): void {
  output = vscode.window.createOutputChannel("Repotory", { log: true });
  statusItem = vscode.window.createStatusBarItem(vscode.StatusBarAlignment.Left, 60);
  statusItem.text = "$(nest) Repotory";
  statusItem.tooltip = "Repotory: click to load the session context";
  statusItem.command = "repotory.loadContext";
  void refreshLastHandoff();

  const notes = new NotesSearchProvider();
  void refreshScope(notes);
  context.subscriptions.push(
    vscode.workspace.onDidChangeWorkspaceFolders(() => void refreshScope(notes)),
  );

  const handoffTimer = setInterval(() => void refreshLastHandoff(), 60_000);
  context.subscriptions.push({ dispose: () => clearInterval(handoffTimer) });

  const register = (id: string, fn: () => Promise<void>) =>
    context.subscriptions.push(
      vscode.commands.registerCommand(id, async () => {
        try {
          await fn();
        } catch (err) {
          void vscode.window.showErrorMessage(
            `Repotory: ${err instanceof Error ? err.message : String(err)}`,
          );
        }
      }),
    );

  register("repotory.loadContext", loadContext);
  register("repotory.handoff", writeHandoff);
  register("repotory.search", () => searchKnowledge(notes));
  register("repotory.registerMcp", registerMcp);

  context.subscriptions.push(
    statusItem,
    vscode.workspace.registerTextDocumentContentProvider(
      "repotory-notes",
      new (class implements vscode.TextDocumentContentProvider {
        provideTextDocumentContent(uri: vscode.Uri): string {
          return Buffer.from(uri.query, "base64").toString("utf8");
        }
      })(),
    ),
    vscode.window.createTreeView("repotory.notesSearch", { treeDataProvider: notes }),
    { dispose: () => mcp?.stop() },
  );

  statusItem.show();
  void pingHeadless().then((ok) => {
    if (!ok) {
      statusItem.tooltip =
        "Headless service not reachable at " +
        serverUrl() +
        " — commands still work through repotory-mcp";
    }
  });
}

export function deactivate(): void {
  mcp?.stop();
}
