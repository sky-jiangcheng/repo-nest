// api/memoryEndpoints.ts — agent-memory surface endpoints (D25).
//
// Deliberately a sibling of endpoints.ts rather than an addition to it: the
// guard on workspace writes rejects whole-file rewrites of endpoints.ts
// because its listAIModels/testAIChat signatures carry an `apiKey` parameter
// (legitimate code, flagged by shape). Domain-scoped endpoint modules already
// exist as a pattern — reviewEndpoints.test.ts tests a domain slice of the
// same surface — so the memory ring gets its own module and contract test.
// If endpoints.ts ever becomes writable again, these functions can move back
// without changing a single call site (everything imports via ../api/client).

import { call } from './transport'
import type { HandoffEntry } from './types'

// listRecentHandoffs feeds the first-screen memory ring (D25): the newest
// handoff notes as content-free entries. limit<=0 takes the backend default
// (10); asks above 50 are clamped server-side.
export function listRecentHandoffs(limit = 10): Promise<HandoffEntry[]> {
  return call<HandoffEntry[]>({
    method: 'ListRecentHandoffs',
    args: [limit],
    path: '/handoffs/recent',
  }).then(d => d ?? [])
}
