// Thin HTTP helper for the RepoNest headless service (reponest server).
// The headless API is loopback-only by design (see internal/httpapi/server.go).
import * as http from "node:http";

export function getJson(base: string, path: string): Promise<unknown> {
  return new Promise((resolve, reject) => {
    const req = http.request(base + path, { method: "GET" }, (res) => {
      let data = "";
      res.setEncoding("utf8");
      res.on("data", (chunk: string) => {
        data += chunk;
      });
      res.on("end", () => {
        try {
          resolve(JSON.parse(data));
        } catch {
          reject(new Error(`invalid JSON from ${path}`));
        }
      });
    });
    req.on("error", reject);
    req.setTimeout(4000, () => {
      req.destroy(new Error(`timeout: ${path}`));
    });
    req.end();
  });
}

export function postJson(base: string, path: string, body: unknown): Promise<unknown> {
  const payload = JSON.stringify(body);
  return new Promise((resolve, reject) => {
    const req = http.request(
      base + path,
      {
        method: "POST",
        headers: { "Content-Type": "application/json", "Content-Length": Buffer.byteLength(payload) },
      },
      (res) => {
        let data = "";
        res.setEncoding("utf8");
        res.on("data", (chunk: string) => {
          data += chunk;
        });
        res.on("end", () => {
          try {
            resolve(JSON.parse(data));
          } catch {
            reject(new Error(`invalid JSON from ${path}`));
          }
        });
      },
    );
    req.on("error", reject);
    req.setTimeout(4000, () => {
      req.destroy(new Error(`timeout: ${path}`));
    });
    req.end(payload);
  });
}

export interface NoteHit {
  id?: number;
  title?: string;
  content?: string;
  [key: string]: unknown;
}

/** Tolerant extraction: the search endpoint may return hits in several shapes. */
export function extractNotes(payload: unknown): NoteHit[] {
  const hits = (payload as { hits?: unknown } | null)?.hits;
  if (Array.isArray(hits)) return hits as NoteHit[];
  const notes = (hits as { notes?: unknown } | null)?.notes;
  if (Array.isArray(notes)) return notes as NoteHit[];
  if (Array.isArray(payload)) return payload as NoteHit[];
  return [];
}

export function excerpt(text: unknown, max = 90): string {
  if (typeof text !== "string") return "";
  const flat = text.replace(/\s+/g, " ").trim();
  return flat.length > max ? `${flat.slice(0, max - 1)}…` : flat;
}
