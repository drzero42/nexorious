# Distributed Rate Limiter for IGDB

## Overview

Replace the per-instance token bucket rate limiter with a NATS KV-backed distributed implementation. All workers share a single 4 req/s limit for IGDB API calls, eliminating the need for manually dividing limits across instances.

## Architecture

```
┌─────────────┐     ┌─────────────┐     ┌─────────────┐
│  Worker 1   │     │  Worker 2   │     │   API       │
│  (IGDBSvc)  │     │  (IGDBSvc)  │     │  (IGDBSvc)  │
└──────┬──────┘     └──────┬──────┘     └──────┬──────┘
       │                   │                   │
       │    CAS read/write │                   │
       └───────────┬───────┴───────────────────┘
                   ▼
          ┌────────────────┐
          │   NATS KV      │
          │ rate-limiters  │
          │   key: igdb    │
          └────────────────┘
```

## Key Decisions

| Aspect | Decision |
|--------|----------|
| Coordination mechanism | NATS KV with CAS (compare-and-swap) |
| Contention handling | Retry loop with jitter (5-50ms) |
| Token refill | Lazy refill on access |
| Failure mode | Fail closed (block requests if NATS unavailable) |
| API surface | Drop-in replacement class |
| KV structure | Single `rate-limiters` bucket, key per resource |
| Serialization | JSON |
| Initialization | Lazy (first worker creates bucket/key) |

## State Model

### NATS KV Configuration

- **Bucket:** `rate-limiters` (created lazily on first access)
- **Key:** `igdb`

### Value Format (JSON)

```json
{
  "tokens": 4.0,
  "last_refill_at": 1703520000.123
}
```

- `tokens`: Float representing available tokens (0.0 to burst capacity)
- `last_refill_at`: Unix timestamp (float, seconds with millisecond precision)

### Token Refill Calculation

On each access:

```python
elapsed = now - last_refill_at
tokens_to_add = elapsed * requests_per_second
new_tokens = min(tokens + tokens_to_add, burst_capacity)
```

### CAS Operation Flow

1. `GET` key returns value + revision number
2. Calculate refill, attempt to decrement token
3. `UPDATE` with revision succeeds only if revision matches
4. On revision mismatch (conflict), retry with jitter

## Class Interface

### `DistributedTokenBucketRateLimiter`

Location: `backend/app/utils/rate_limiter.py`

**Constructor:**

```python
def __init__(
    self,
    nats_client: nats.NATS,
    resource_name: str,  # e.g., "igdb"
    config: RateLimitConfig,
    max_cas_retries: int = 10,
    cas_retry_base_ms: int = 5,
    cas_retry_max_ms: int = 50,
)
```

**Methods (matching existing `TokenBucketRateLimiter` interface):**

- `async def acquire(self, tokens: float = 1.0) -> bool` - Non-blocking, returns True if tokens acquired
- `async def wait_for_tokens(self, tokens: float = 1.0, timeout: float | None = None) -> bool` - Blocking with optional timeout
- `async def get_status(self) -> dict` - Returns tokens available, utilization, etc.

## Error Handling

### CAS Contention

When `UPDATE` fails due to revision mismatch:

1. Wait random delay: `random.uniform(cas_retry_base_ms, cas_retry_max_ms)` milliseconds
2. Retry from GET
3. After `max_cas_retries` (default 10), raise `RateLimitError`

### NATS Unavailability (Fail Closed)

When NATS operations raise connection errors:

- `acquire()` returns `False`
- `wait_for_tokens()` waits and retries until timeout, then returns `False`
- Logs warning for observability

No IGDB requests are made when coordination is unavailable.

### Lazy Initialization Errors

If bucket/key creation fails, retry with backoff. After retries exhausted, fail closed.

## Configuration

### New Settings

Add to `backend/app/core/config.py`:

```python
RATE_LIMITER_MODE: str = "distributed"  # "local" or "distributed"
RATE_LIMITER_NATS_BUCKET: str = "rate-limiters"
RATE_LIMITER_CAS_MAX_RETRIES: int = 10
RATE_LIMITER_CAS_RETRY_BASE_MS: int = 5
RATE_LIMITER_CAS_RETRY_MAX_MS: int = 50
```

Existing IGDB settings remain unchanged:

- `IGDB_REQUESTS_PER_SECOND` (4.0)
- `IGDB_BURST_CAPACITY` (8)

### Docker Compose Changes

Remove per-worker rate limit overrides:

```yaml
# Remove these from workers:
# IGDB_REQUESTS_PER_SECOND=1.0
# IGDB_BURST_CAPACITY=2
```

All instances share the global 4 req/s limit via NATS.

## Integration

### IGDBService Changes

In `backend/app/services/igdb/service.py`:

- Replace `TokenBucketRateLimiter` instantiation with `DistributedTokenBucketRateLimiter`
- Pass the NATS client (available via broker module)
- No other changes needed (same interface)

## Testing Strategy

### Unit Tests

Add to `backend/app/tests/test_rate_limiter.py`:

1. Basic acquisition - Single worker acquires tokens, state updates correctly
2. Token refill - Tokens refill based on elapsed time
3. CAS conflict simulation - Mock revision mismatch, verify retry with jitter
4. Exhaustion - Bucket depletes, acquire returns False
5. Lazy initialization - First access creates bucket and key

### Integration Tests

New file `backend/app/tests/test_distributed_rate_limiter.py`:

1. Multi-worker simulation - Multiple coroutines competing for tokens, verify total requests stay within limit
2. NATS unavailability - Mock connection failure, verify fail-closed behavior
3. Recovery after NATS outage - NATS comes back, operations resume

### Test Infrastructure

- Use NATS container from testcontainers
- Create isolated bucket per test
- Clean up buckets after tests

### Existing Tests

`test_rate_limited_igdb_service.py` continues passing (may mock distributed rate limiter or use local mode).
