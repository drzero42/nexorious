# Playwright MCP Automation Guide

## Quick Reference for Claude Code

### Key Principle
**Never rely on MCP Playwright element refs** - they become stale immediately due to SvelteKit reactivity and Vite HMR.

### Reliable Interaction Pattern

1. **Add test IDs to components first**:
```svelte
<input data-testid="login-username" />
<button data-testid="login-submit">Submit</button>
```

2. **Use JavaScript navigation for interactions**:
```javascript
// Single interaction
await browser_navigate({ url: `javascript:document.querySelector('[data-testid="logout-button"]').click()` });

// Complex form filling
await browser_navigate({ 
  url: `javascript:
    document.querySelector('[data-testid="login-username"]').value='admin';
    document.querySelector('[data-testid="login-password"]').value='password';
    document.querySelector('[data-testid="login-submit"]').click();
  `
});
```

### What NOT to Do
- ❌ `browser_click` with refs - refs become stale instantly
- ❌ `browser_type` with refs - same issue
- ❌ `browser_evaluate` - function doesn't work properly in MCP
- ❌ Multiple individual key presses - slow and unreliable

### What TO Do
- ✅ Add `data-testid` attributes to interactive elements
- ✅ Use `browser_navigate` with JavaScript URLs
- ✅ Combine multiple actions in single JavaScript call
- ✅ Trigger input events for reactive frameworks: `el.dispatchEvent(new Event('input', {bubbles: true}))`

### Common Patterns

**Login**:
```javascript
javascript:
  const user = document.querySelector('[data-testid="login-username"]');
  const pass = document.querySelector('[data-testid="login-password"]');
  user.value='admin'; 
  pass.value='password';
  user.dispatchEvent(new Event('input', {bubbles: true}));
  pass.dispatchEvent(new Event('input', {bubbles: true}));
  document.querySelector('[data-testid="login-submit"]').click();
```

**Logout**:
```javascript
javascript:document.querySelector('[data-testid="logout-button"]').click()
```

**Form Submission**:
```javascript
javascript:document.querySelector('[data-testid="form-submit"]').click()
```

### Remember
- Always add test IDs before attempting automation
- Use `browser_snapshot` to see page state, not for getting refs
- Combine multiple actions into single JavaScript calls for efficiency
- This approach works reliably with reactive frameworks like SvelteKit