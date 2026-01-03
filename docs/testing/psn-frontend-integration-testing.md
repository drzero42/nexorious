# PSN Frontend Integration Testing

## Test Environment Setup

1. Ensure backend PSN implementation is deployed and running
2. Ensure you have a valid PSN account
3. Obtain a fresh NPSSO token from https://ca.account.sony.com/api/v1/ssocookie

## Test Cases

### TC1: PSN Configuration - Happy Path

**Steps:**
1. Navigate to Settings page
2. Locate PlayStation Network card
3. Verify card shows "Not Configured" badge
4. Click "How do I get my NPSSO token?" accordion
5. Follow instructions to obtain NPSSO token
6. Paste token into input field
7. Click "Verify & Connect"

**Expected:**
- Loading spinner appears during verification
- Success toast: "PlayStation Network connected successfully"
- Card updates to show "Connected" badge
- Connected state displays PSN Online ID, Account ID, and Region
- Disconnect button appears

### TC2: PSN Configuration - Invalid Token

**Steps:**
1. Navigate to Settings page
2. Enter invalid token (e.g., too short, wrong format)
3. Click "Verify & Connect"

**Expected:**
- Error message appears below input: "NPSSO token must be exactly 64 characters" or "Invalid NPSSO token format"
- Card remains in not configured state

### TC3: PSN Configuration - Token Expired

**Prerequisite:** PSN configured with expired token (backend mock or wait 2 months)

**Steps:**
1. Navigate to Settings page
2. Observe PSN card

**Expected:**
- Card shows "Token Expired" badge (yellow)
- Warning alert: "Your NPSSO token has expired. Please enter a new token to continue syncing."
- Input field available to enter new token
- Previous account info still visible

### TC4: PSN Sync - Manual Trigger

**Prerequisite:** PSN configured successfully

**Steps:**
1. Navigate to Sync page (/sync)
2. Locate PlayStation Network sync card
3. Click "Sync Now" button

**Expected:**
- Sync starts immediately
- Redirects to /sync/psn page
- Job progress card appears
- Games begin appearing in collection with PS4/PS5 platforms

### TC5: PSN Sync - Update Settings

**Prerequisite:** PSN configured successfully

**Steps:**
1. Navigate to Sync page (/sync)
2. Locate PlayStation Network sync card
3. Change frequency from "Manual" to "Daily"
4. Toggle "Auto-add" to ON
5. Observe sync settings

**Expected:**
- Settings update successfully
- Success toast: "Sync settings updated successfully"
- Card reflects new settings immediately

### TC6: PSN Disconnect

**Prerequisite:** PSN configured successfully

**Steps:**
1. Navigate to Settings page
2. Click "Disconnect" button on PSN card
3. Confirm disconnect in dialog

**Expected:**
- Confirmation dialog appears with warning
- After confirming, success toast: "PlayStation Network disconnected"
- Card returns to "Not Configured" state
- Sync settings preserved (still visible on sync page)

### TC7: Multi-Platform Games

**Prerequisite:** PSN configured with games that have both PS4 and PS5 versions

**Steps:**
1. Trigger PSN sync
2. Wait for sync to complete
3. Navigate to game collection
4. Find a cross-gen game (e.g., Spider-Man)
5. View game details

**Expected:**
- Game appears with BOTH "PlayStation 4" and "PlayStation 5" platform tags
- Separate entries for each platform in platforms list

### TC8: Type Safety Verification

**Steps:**
1. Open browser developer console
2. Navigate through PSN settings/sync pages
3. Trigger PSN operations
4. Monitor console for errors

**Expected:**
- Zero console errors
- Zero TypeScript errors
- Zero React errors
- All network requests succeed (or fail gracefully with user messages)

## Performance Testing

### PT1: Load Time

**Steps:**
1. Navigate to Settings page
2. Measure time to render PSN card

**Expected:**
- Card renders in <500ms
- No layout shift after hydration

### PT2: API Response Handling

**Steps:**
1. Configure PSN (network tab open)
2. Observe API response times

**Expected:**
- /sync/psn/configure responds in <2s
- /sync/psn/status responds in <500ms
- Loading states shown during requests

## Accessibility Testing

### AT1: Keyboard Navigation

**Steps:**
1. Navigate to Settings page using only keyboard
2. Tab through PSN card elements
3. Use Enter/Space to interact

**Expected:**
- All interactive elements focusable
- Focus visible (outline or highlight)
- Can complete configuration flow with keyboard only

### AT2: Screen Reader Testing

**Steps:**
1. Enable screen reader (NVDA, JAWS, or VoiceOver)
2. Navigate PSN card
3. Interact with form elements

**Expected:**
- Labels announced correctly
- Error messages announced
- Button states announced (loading, disabled)

## Edge Cases

### EC1: Network Failure During Configuration

**Steps:**
1. Disconnect internet
2. Attempt PSN configuration
3. Reconnect internet

**Expected:**
- Error toast: "Could not connect to PlayStation Network. Please try again."
- Form remains in error state
- User can retry after reconnecting

### EC2: Concurrent Configuration Attempts

**Steps:**
1. Open settings in two browser tabs
2. Configure PSN in first tab
3. Attempt to configure in second tab

**Expected:**
- Second tab detects first configuration
- Both tabs show connected state after refresh
- No duplicate configurations created

## Browser Compatibility

Test on:
- [ ] Chrome (latest)
- [ ] Firefox (latest)
- [ ] Safari (latest)
- [ ] Edge (latest)

## Mobile Testing

Test on:
- [ ] iOS Safari
- [ ] Android Chrome
- [ ] Responsive design (320px - 1920px)
