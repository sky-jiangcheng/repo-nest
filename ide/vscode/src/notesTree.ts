// Knowledge search sidebar (tree view "repotory.notesSearch").
// Thin client: renders hits from repotory_notes_search (MCP) or /api/search
// (headless HTTP) — all business logic stays in the Repotory service layer.
import * as vscode from "vscode";
import { excerpt, extractNotes, type NoteHit } from "./headless.js";

export const MAX_HITS = 50;

/** Parse the JSON payload printed by makeJSONResult("notes_search", …). */
export function parseSearchHits(text: string): NoteHit[] {
  const start = text.indexOf("{");
  if (start < 0) return [];
  try {
    return extractNotes(JSON.parse(text.slice(start)));
  } catch {
    return [];
  }
}

export class NotesSearchProvider implements vscode.TreeDataProvider<NoteItem> {
  private hits: NoteHit[] = [];
  private scope = "";
  private hint = "Run the search command to query the knowledge base";
  private readonly emitter = new vscode.EventEmitter<void>();
  readonly onDidChangeTreeData = this.emitter.event;

  getTreeItem(element: NoteItem): vscode.TreeItem {
    return element;
  }

  /**
   * Announce what the view is scoped to. An idle sidebar that says only "run the
   * search command" is indistinguishable from a broken one, so once the workspace
   * resolves it states the project and repository it will search within.
   */
  setScope(scope: string): void {
    this.scope = scope;
    this.emitter.fire();
  }

  getChildren(): NoteItem[] {
    if (this.hits.length === 0) {
      const label = this.scope ? `${this.scope} — ${this.hint}` : this.hint;
      return [new NoteItem(label, undefined)];
    }
    const items = this.hits.slice(0, MAX_HITS).map((hit) => {
      const title =
        typeof hit.title === "string" && hit.title
          ? hit.title
          : excerpt(hit.content, 40) || `note ${String(hit.id ?? "?")}`;
      return new NoteItem(title, hit);
    });
    if (this.hits.length > MAX_HITS) {
      items.push(new NoteItem(`…${this.hits.length - MAX_HITS} more (refine the query)`, undefined));
    }
    return items;
  }

  setHint(hint: string): void {
    this.hits = [];
    this.hint = hint;
    this.emitter.fire();
  }

  setHits(hits: NoteHit[], query: string): void {
    this.hits = hits;
    if (hits.length === 0) this.hint = `No results for "${query}"`;
    this.emitter.fire();
  }
}

export class NoteItem extends vscode.TreeItem {
  constructor(label: string, hit?: NoteHit) {
    super(label, vscode.TreeItemCollapsibleState.None);
    if (!hit) {
      this.contextValue = "hint";
      return;
    }
    this.description = excerpt(hit.content, 60);
    this.tooltip = typeof hit.content === "string" ? hit.content : undefined;
    const body = typeof hit.content === "string" ? hit.content : "";
    this.command = {
      command: "vscode.open",
      title: "Read Note",
      arguments: [
        vscode.Uri.from({
          scheme: "repotory-notes",
          path: `/note-${String(hit.id ?? "0")}.md`,
          query: Buffer.from(body || `note ${String(hit.id ?? "?")} (content unavailable)`).toString("base64"),
        }),
      ],
    };
  }
}
