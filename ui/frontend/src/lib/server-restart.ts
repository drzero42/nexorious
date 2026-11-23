/**
 * The server restarts itself at the end of a restore (#1341). These helpers
 * let a restore dialog wait for the new server: read `/health` before the
 * request, then wait until it answers with a different instance id.
 */
import { ApiErrorException } from '@/api/client';

export interface InstanceHealth {
  status: string;
  instance_id?: string;
  restore_in_progress?: boolean;
}

/** Reads `/health`; null when the server does not answer. */
export async function readHealth(timeoutMs = 5_000): Promise<InstanceHealth | null> {
  try {
    const res = await fetch('/health', {
      cache: 'no-store',
      signal: AbortSignal.timeout(timeoutMs),
    });
    if (!res.ok) return null;
    return (await res.json()) as InstanceHealth;
  } catch {
    return null;
  }
}

function asRecord(v: unknown): Record<string, unknown> | undefined {
  return typeof v === 'object' && v !== null ? (v as Record<string, unknown>) : undefined;
}

/** The body of a restore response, from its result or from the error it threw. */
export function restoreBody(result: unknown, err: unknown): Record<string, unknown> | undefined {
  if (err instanceof ApiErrorException) return asRecord(err.details);
  return asRecord(result);
}

/**
 * Whether a restore's outcome means the server is restarting, or may be. A body
 * from the restore handler says so itself; without one, any 5xx (a panic, a
 * proxy that gave up) and a request that got no answer may have started it.
 */
export function mustWaitForRestart(result: unknown, err: unknown): boolean {
  const body = restoreBody(result, err);
  if (typeof body?.restarting === 'boolean') return body.restarting;
  if (err === undefined) return false;
  if (err instanceof ApiErrorException) return err.status >= 500;
  return true;
}

export class RestartTimeoutError extends Error {
  constructor() {
    super('The server did not come back after the restore. Check the server logs.');
    this.name = 'RestartTimeoutError';
  }
}

export interface WaitOptions {
  /** How long to wait once the old server stops reporting a restore in progress. */
  graceMs?: number;
  intervalMs?: number;
  read?: () => Promise<InstanceHealth | null>;
  sleep?: (ms: number) => Promise<void>;
  now?: () => number;
}

/**
 * Polls `/health` until a server with an instance id other than oldId answers.
 * Without an oldId it settles for the first answer that reports no restore in
 * progress. The grace period starts again each time the old server reports a
 * restore still in progress, since a restore can run for many minutes after a
 * proxy has given up on the request.
 */
export async function waitForNewInstance(
  oldId: string | undefined,
  {
    graceMs = 120_000,
    intervalMs = 1_000,
    read = readHealth,
    sleep = (ms) => new Promise((resolve) => setTimeout(resolve, ms)),
    now = Date.now,
  }: WaitOptions = {},
): Promise<InstanceHealth> {
  let deadline = now() + graceMs;
  for (;;) {
    const h = await read();
    if (h) {
      if (oldId && h.instance_id && h.instance_id !== oldId) return h;
      if (!oldId && !h.restore_in_progress) return h;
      if (h.restore_in_progress) deadline = now() + graceMs;
    }
    if (now() > deadline) throw new RestartTimeoutError();
    await sleep(intervalMs);
  }
}

const OUTCOME_KEY = 'nexorious.restoreOutcome';

export interface RestoreOutcome {
  kind: 'success' | 'error';
  message: string;
}

/** Keeps a restore's outcome across the page load that follows the restart. */
export function stashRestoreOutcome(outcome: RestoreOutcome): void {
  try {
    sessionStorage.setItem(OUTCOME_KEY, JSON.stringify(outcome));
  } catch {
    // Storage unavailable: the outcome is not shown after the reload.
  }
}

/** Returns and forgets the outcome stashed before the restart, if any. */
export function takeRestoreOutcome(): RestoreOutcome | null {
  try {
    const raw = sessionStorage.getItem(OUTCOME_KEY);
    if (!raw) return null;
    sessionStorage.removeItem(OUTCOME_KEY);
    const o = JSON.parse(raw) as Partial<RestoreOutcome>;
    if ((o.kind === 'success' || o.kind === 'error') && typeof o.message === 'string') {
      return { kind: o.kind, message: o.message };
    }
  } catch {
    // Unreadable: nothing to show.
  }
  return null;
}
