import { describe, it, expect, beforeEach } from 'vitest';
import { ApiErrorException } from '@/api/client';
import {
  mustWaitForRestart,
  RestartTimeoutError,
  stashRestoreOutcome,
  takeRestoreOutcome,
  waitForNewInstance,
} from './server-restart';
import type { InstanceHealth } from './server-restart';

describe('mustWaitForRestart', () => {
  it('waits after a success that restarts', () => {
    expect(mustWaitForRestart({ success: true, restarting: true }, undefined)).toBe(true);
  });

  it('waits after a failure that restarts', () => {
    const err = new ApiErrorException('restore failed', 500, { restarting: true, error: 'x' });
    expect(mustWaitForRestart(undefined, err)).toBe(true);
  });

  it('does not wait after a refusal that keeps the server running', () => {
    const err = new ApiErrorException('in progress', 409, { restarting: false });
    expect(mustWaitForRestart(undefined, err)).toBe(false);
    // A 503 from the restore handler (pg_dump missing) keeps it running too.
    const unavailable = new ApiErrorException('no pg_dump', 503, { restarting: false });
    expect(mustWaitForRestart(undefined, unavailable)).toBe(false);
  });

  it('waits when no answer arrived', () => {
    expect(mustWaitForRestart(undefined, new TypeError('Failed to fetch'))).toBe(true);
  });

  it('waits after a 5xx that did not come from the restore handler', () => {
    expect(mustWaitForRestart(undefined, new ApiErrorException('Bad Gateway', 504))).toBe(true);
    expect(mustWaitForRestart(undefined, new ApiErrorException('oops', 500, 'plain text'))).toBe(
      true,
    );
  });

  it('does not wait after a 4xx that did not come from the restore handler', () => {
    expect(mustWaitForRestart(undefined, new ApiErrorException('Forbidden', 403))).toBe(false);
  });
});

// A fake clock and a scripted /health: each read advances time by one poll.
function scripted(answers: Array<InstanceHealth | null>, stepMs = 1_000) {
  let t = 0;
  let i = 0;
  return {
    read: async () => answers[Math.min(i++, answers.length - 1)],
    sleep: async () => {
      t += stepMs;
    },
    now: () => t,
    reads: () => i,
  };
}

const old = (restoring = false): InstanceHealth => ({
  status: 'ok',
  instance_id: 'old',
  restore_in_progress: restoring,
});
const fresh: InstanceHealth = { status: 'ok', instance_id: 'new', restore_in_progress: false };

describe('waitForNewInstance', () => {
  it('returns once a new instance id answers', async () => {
    const s = scripted([old(true), null, null, fresh]);
    await expect(waitForNewInstance('old', { ...s, graceMs: 10_000 })).resolves.toEqual(fresh);
    expect(s.reads()).toBe(4);
  });

  it('gives up when no new instance appears within the grace period', async () => {
    const s = scripted([null]);
    await expect(waitForNewInstance('old', { ...s, graceMs: 5_000 })).rejects.toBeInstanceOf(
      RestartTimeoutError,
    );
  });

  it('keeps waiting while the old server reports a restore in progress', async () => {
    // Twenty polls of "still restoring" far outlast a 3 s grace period.
    const s = scripted([...Array.from({ length: 20 }, () => old(true)), null, fresh]);
    await expect(waitForNewInstance('old', { ...s, graceMs: 3_000 })).resolves.toEqual(fresh);
  });

  it('does not take the old server for the new one', async () => {
    const s = scripted([old()]);
    await expect(waitForNewInstance('old', { ...s, graceMs: 3_000 })).rejects.toBeInstanceOf(
      RestartTimeoutError,
    );
  });

  it('without an old id settles for a server that is not restoring', async () => {
    const s = scripted([old(true), null, fresh]);
    await expect(waitForNewInstance(undefined, { ...s, graceMs: 10_000 })).resolves.toEqual(fresh);
  });
});

describe('restore outcome across the reload', () => {
  beforeEach(() => sessionStorage.clear());

  it('is shown once', () => {
    stashRestoreOutcome({ kind: 'error', message: 'Restore failed' });
    expect(takeRestoreOutcome()).toEqual({ kind: 'error', message: 'Restore failed' });
    expect(takeRestoreOutcome()).toBeNull();
  });

  it('ignores a malformed entry', () => {
    sessionStorage.setItem('nexorious.restoreOutcome', '{"kind":"other"}');
    expect(takeRestoreOutcome()).toBeNull();
  });
});
