import { describe, it, expect } from 'vitest';
import { isJobInProgress, JobStatus } from './jobs';
import type { Job } from './jobs';

function makeJob(status: JobStatus, isTerminal: boolean): Job {
  return { status, isTerminal } as Job;
}

describe('isJobInProgress', () => {
  it('returns true for a pending non-terminal job', () => {
    expect(isJobInProgress(makeJob(JobStatus.PENDING, false))).toBe(true);
  });

  it('returns true for a processing non-terminal job', () => {
    expect(isJobInProgress(makeJob(JobStatus.PROCESSING, false))).toBe(true);
  });

  it('returns false for a completed terminal job', () => {
    expect(isJobInProgress(makeJob(JobStatus.COMPLETED, true))).toBe(false);
  });

  it('returns false for a failed terminal job', () => {
    expect(isJobInProgress(makeJob(JobStatus.FAILED, true))).toBe(false);
  });

  it('returns false for a cancelled terminal job', () => {
    expect(isJobInProgress(makeJob(JobStatus.CANCELLED, true))).toBe(false);
  });
});
