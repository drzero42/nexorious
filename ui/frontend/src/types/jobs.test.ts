import { describe, it, expect } from 'vitest';
import { isJobInProgress, JobStatus } from './jobs';
import type { Job } from './jobs';

function makeJob(status: JobStatus, isTerminal: boolean): Job {
  return { status, isTerminal } as Job;
}

describe('isJobInProgress', () => {
  // isJobInProgress is `!job.isTerminal` — the status is not inspected, so the
  // only meaningful axis is the isTerminal flag.
  it.each([
    [false, true],
    [true, false],
  ])('isTerminal=%s → %s', (isTerminal, expected) => {
    expect(isJobInProgress(makeJob(JobStatus.PROCESSING, isTerminal))).toBe(expected);
  });
});
