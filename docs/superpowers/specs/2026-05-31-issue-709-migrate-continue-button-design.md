# Issue #709 — Migration UI: Show Continue Button After Success

## Problem

When migrations run successfully, the `complete` SSE event handler immediately calls `checkStatusAndAct()`, which redirects to `/` as soon as the server reports `state=ready`. The log window disappears before the user can read the output.

The failure path already handles this correctly — it leaves the log visible and re-enables the button with "Retry". The success path does not apply the same pattern.

## Design

**One new function, two changed lines** — all in `ui/migrate/index.html`.

### `showSuccess()`

A new function called from the `complete` SSE event handler instead of `checkStatusAndAct()`:

```js
function showSuccess() {
  var btn = document.getElementById('btn');
  var status = document.getElementById('status');
  status.textContent = 'Migrations complete — click Continue to proceed';
  status.className = 'meta meta--success';
  btn.textContent = 'Continue';
  btn.onclick = function() {
    btn.disabled = true;
    checkStatusAndAct()
      .then(function() { startPolling(); })
      .catch(function() {
        status.textContent = 'Could not verify migration status. Refresh to retry.';
        status.className = 'meta meta--error';
        btn.disabled = false;
      });
  };
  btn.disabled = false;
}
```

### `complete` handler change

Replace the `checkStatusAndAct()` call in the `complete` handler with `showSuccess()`. Remove the chained `.then(() => startPolling())` and `.catch(...)` from the handler — they move into the Continue click handler:

```js
es.addEventListener('complete', function() {
  completed = true;
  es.close();
  showSuccess();
});
```

### Behaviour after the change

| State | Button | Status line |
|---|---|---|
| Idle (page load) | Run Migrations | (empty) |
| Running | Run Migrations (disabled) | Running migrations… |
| Success | Continue (enabled) | Migrations complete — click Continue to proceed |
| Continue clicked | Continue (disabled) | (polling until redirect) |
| Failure | Retry (enabled) | Migration failed: … |

### Polling fallback

When the user clicks Continue, `checkStatusAndAct()` fires and `startPolling()` is chained as before, handling the brief window where the server may still report `state=migrating` before transitioning to `ready`.

### No auto-redirect timeout

The Continue button is strictly manual. No timer, no auto-advance.

## Files Changed

- `ui/migrate/index.html` — only file touched
