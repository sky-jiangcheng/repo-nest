// RepoNest for VS Code — thin client over RepoNest (ADR-0009 step two / TODO M5).
//
// Commands: load session context, write the session handoff and search the
// knowledge base; a status-bar item and a knowledge-search sidebar make the
// product visible inside the IDE. Everything is delegated: read endpoints go
// to the loopback-only headless service when it is up, all tools go through
// reponest-mcp over stdio — zero logic duplication (thin-client discipline).
import * as vscode from "vscode";
import * as path from "node:path";
import { accessSync } from "node:fs";
import { McpStdioClient } from "./mcpClient.js";
import { getJson, postJson } from "./headless.js";
import { NotesSearchProvider, parseSearchHits } from "./notesTree.js";

const CONFIG_SECTION = "reponest";
const SERVER_URL_KEY = "serverUrl";
const PROJECT_ID_KEY = "projectId";
const BINARY_NAMES = ["reponest-mcp", "reponest-mcp.exe"];

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
      "RepoNest: reponest-mcp binary not found on PATH. Install it from Releases, then retry.",
      "Open Releases",
    );
    if (choice === "Open Releases") {
      void vscode.env.openExternal(
        vscode.Uri.parse("https://github.com/sky-jiangcheng/repo-nest/releases"),
      );
    }
    throw new Error("reponest-mcp not found");
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
      statusItem.text = `$(nest) RepoNest · ${at.toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" })}`;
      statusItem.tooltip = `Last handoff: ${handoff.title ?? "session"}\n${at.toLocaleString()}\n\nClick to load the session context`;
    } else {
      statusItem.text = "$(nest) RepoNest";
      statusItem.tooltip = "RepoNest: no session handoff yet. Click to load the session context";
    }
  } catch {
    statusItem.text = "$(nest) RepoNest";
    statusItem.tooltip = "RepoNest: click to load the session context";
  }
}

function withProgress<T>(title: string, task: () => Promise<T>): Thenable<T> {
  return vscode.window.withProgress(
    { location: vscode.ProgressLocation.Notification, title: `RepoNest: ${title}` },
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
  const text = await withProgress("loading session context", () =>
    callTool("reponest_context", args),
  );
  showOutput("Session context", text);
}

async function writeHandoff(): Promise<void> {
  const projectId = configuredProjectId();
  if (projectId === undefined) {
    void vscode.window.showErrorMessage(
      "RepoNest: set reponest.projectId in settings — the handoff tool needs a project id.",
    );
    return;
  }
  const summary = await vscode.window.showInputBox({
    prompt: "RepoNest handoff — one paragraph: what was this session about?",
    placeHolder: "e.g. Wired the FTS5 rebuild path; tests green",
    ignoreFocusOut: true,
  });
  if (!summary) return;

  const askList = async (label: string): Promise<string[]> => {
    const raw = await vscode.window.showInputBox({
      prompt: `RepoNest handoff — ${label} (comma-separated, empty to skip)`,
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

  const noteText = await withProgress("writing session handoff", () =>
    callTool("reponest_handoff", {
      project_id: projectId,
      agent: "vscode-extension",
      summary,
      changes,
      next_steps: nextSteps,
    }),
  );
  showOutput("Handoff recorded", noteText);
  void refreshLastHandoff();
}

async function searchKnowledge(notes: NotesSearchProvider): Promise<void> {
  const query = await vscode.window.showInputBox({
    prompt: "RepoNest — search notes & todos (FTS5)",
    placeHolder: "e.g. deploy hook",
    ignoreFocusOut: true,
  });
  if (!query) return;
  notes.setHint(`Searching "${query}"…`);
  try {
    const text = await withProgress("searching knowledge", () =>
      callTool("reponest_notes_search", { query }),
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
    void vscode.window.showErrorMessage("RepoNest: open a folder first.");
    return;
  }
  const script = vscode.Uri.joinPath(folder.uri, "scripts", "reponest-init", "index.mjs");
  try {
    await vscode.workspace.fs.stat(script);
  } catch {
    const choice = await vscode.window.showWarningMessage(
      "RepoNest: scripts/reponest-init/index.mjs was not found in this workspace — the one-command installer lives in the RepoNest repository.",
      "Open Docs",
    );
    if (choice === "Open Docs") {
      void vscode.env.openExternal(
        vscode.Uri.parse(
          "https://github.com/sky-jiangcheng/repo-nest/blob/master/docs/features/ai-integration.md",
        ),
      );
    }
    return;
  }
  const terminal = vscode.window.createTerminal({ name: "reponest-init", cwd: folder.uri });
  terminal.sendText("node scripts/reponest-init/index.mjs --with-hook");
  terminal.show();
}

// ---------------------------------------------------------------- lifecycle

export function activate(context: vscode.ExtensionContext): void {
  output = vscode.window.createOutputChannel("RepoNest", { log: true });
  statusItem = vscode.window.createStatusBarItem(vscode.StatusBarAlignment.Left, 60);
  statusItem.text = "$(nest) RepoNest";
  statusItem.tooltip = "RepoNest: click to load the session context";
  statusItem.command = "reponest.loadContext";
  void refreshLastHandoff();

  const notes = new NotesSearchProvider();

  const handoffTimer = setInterval(() => void refreshLastHandoff(), 60_000);
  context.subscriptions.push({ dispose: () => clearInterval(handoffTimer) });

  const register = (id: string, fn: () => Promise<void>) =>
    context.subscriptions.push(
      vscode.commands.registerCommand(id, async () => {
        try {
          await fn();
        } catch (err) {
          void vscode.window.showErrorMessage(
            `RepoNest: ${err instanceof Error ? err.message : String(err)}`,
          );
        }
      }),
    );

  register("reponest.loadContext", loadContext);
  register("reponest.handoff", writeHandoff);
  register("reponest.search", () => searchKnowledge(notes));
  register("reponest.registerMcp", registerMcp);

  context.subscriptions.push(
    statusItem,
    vscode.workspace.registerTextDocumentContentProvider(
      "reponest-notes",
      new (class implements vscode.TextDocumentContentProvider {
        provideTextDocumentContent(uri: vscode.Uri): string {
          return Buffer.from(uri.query, "base64").toString("utf8");
        }
      })(),
    ),
    vscode.window.createTreeView("reponest.notesSearch", { treeDataProvider: notes }),
    { dispose: () => mcp?.stop() },
  );

  statusItem.show();
  void pingHeadless().then((ok) => {
    if (!ok) {
      statusItem.tooltip =
        "Headless service not reachable at " +
        serverUrl() +
        " — commands still work through reponest-mcp";
    }
  });
}

export function deactivate(): void {
  mcp?.stop();
}
