<script lang="ts">
 import { auth } from '$lib/stores';
 import { goto } from '$app/navigation';
 import { RouteGuard } from '$lib/components';

 let email = '';
 let username = '';
 let password = '';
 let confirmPassword = '';
 let isLoading = false;
 let error = '';
 let emailValid = false;
 let usernameChecking = false;
 let usernameAvailable = false;
 let passwordStrength = 0;

 async function handleRegister() {
  if (!email || !username || !password || !confirmPassword) {
   error = 'Please fill in all required fields';
   return;
  }

  if (password !== confirmPassword) {
   error = 'Passwords do not match';
   return;
  }

  if (password.length < 8) {
   error = 'Password must be at least 8 characters long';
   return;
  }

  isLoading = true;
  error = '';

  try {
   await auth.register({
    email,
    username,
    password
   });
   goto('/games');
  } catch (err) {
   error = err instanceof Error ? err.message : 'Registration failed';
  } finally {
   isLoading = false;
  }
 }

 function handleKeydown(event: KeyboardEvent) {
  if (event.key === 'Enter') {
   handleRegister();
  }
 }

 // Email validation
 function validateEmail(email: string): boolean {
  const emailRegex = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;
  return emailRegex.test(email);
 }

 // Password strength calculation (0-5)
 function calculatePasswordStrength(password: string): number {
  let strength = 0;
  if (password.length >= 8) strength++;
  if (password.length >= 12) strength++;
  if (/[a-z]/.test(password)) strength++;
  if (/[A-Z]/.test(password)) strength++;
  if (/[0-9]/.test(password)) strength++;
  if (/[^A-Za-z0-9]/.test(password)) strength++;
  return Math.min(strength, 5);
 }

 // Username availability checking (debounced)
 let usernameCheckTimeout: NodeJS.Timeout;
 
 async function checkUsernameAvailability(username: string) {
  if (username.length < 3) {
   usernameChecking = false;
   usernameAvailable = false;
   return;
  }
  
  usernameChecking = true;
  
  // Simple timeout to simulate API call
  await new Promise(resolve => setTimeout(resolve, 500));
  
  // For demo purposes, mark username as available if it doesn't contain "admin" or "test"
  usernameAvailable = !username.toLowerCase().includes('admin') && !username.toLowerCase().includes('test');
  usernameChecking = false;
 }

 function debouncedUsernameCheck(username: string) {
  clearTimeout(usernameCheckTimeout);
  usernameCheckTimeout = setTimeout(() => {
   checkUsernameAvailability(username);
  }, 500);
 }

 // Reactive statements for validation
 $: emailValid = email.length > 0 && validateEmail(email);
 $: passwordStrength = calculatePasswordStrength(password);
 $: if (username) debouncedUsernameCheck(username);
</script>

<svelte:head>
 <title>Register - Nexorious</title>
</svelte:head>

<RouteGuard requireAuth={false}>
<div>
 <div>
  <div>
   <!-- Header -->
   <div>
    <div>
     <h1>Nexorious</h1>
     <p>Game Collection Manager</p>
    </div>
   </div>
   
   <div>
    <!-- Form Header -->
    <div>
     <h2>
      Create Account
     </h2>
     <p>
      Join Nexorious to start managing your game collection
     </p>
    </div>

    {#if error}
     <div>
      {error}
     </div>
    {/if}

    <form on:submit|preventDefault={handleRegister}>
     <!-- Email field -->
     <div>
      <label for="email">
       Email Address
      </label>
      <div>
       <input
        id="email"
        type="email"
        bind:value={email}
        on:keydown={handleKeydown}
        required
        placeholder="Enter your email address"
       />
       {#if emailValid}
        <div>
         <div>
✓
         </div>
        </div>
       {/if}
      </div>
     </div>

     <!-- Username field -->
     <div>
      <label for="username">
       Username
      </label>
      <div>
       <input
        id="username"
        type="text"
        bind:value={username}
        on:keydown={handleKeydown}
        required
        placeholder="Choose a unique username"
       />
       {#if usernameChecking}
        <div>
         <div>
          <span>?</span>
         </div>
        </div>
       {:else if username.length >= 3 && usernameAvailable}
        <div>
         <div>
✓
         </div>
        </div>
       {:else if username.length >= 3 && !usernameAvailable}
        <div>
         <div>
✗
         </div>
        </div>
       {/if}
      </div>
     </div>

     <!-- Password field -->
     <div>
      <label for="password">
       Password
      </label>
      <input
       id="password"
       type="password"
       bind:value={password}
       on:keydown={handleKeydown}
       required
       placeholder="Enter a strong password"
      />
      <!-- Password strength meter -->
      <div>
       <div>
        <div 
         style="width: {(passwordStrength / 5) * 100}%"
        ></div>
       </div>
       <p>
        {#if passwordStrength < 3}
         Weak - Add numbers and symbols
        {:else if passwordStrength < 5}
         Medium - Add more characters
        {:else}
         Strong password
        {/if}
       </p>
      </div>
     </div>

     <!-- Confirm Password field -->
     <div>
      <label for="confirmPassword">
       Confirm Password
      </label>
      <input
       id="confirmPassword"
       type="password"
       bind:value={confirmPassword}
       on:keydown={handleKeydown}
       required
       placeholder="Confirm your password"
      />
     </div>

     <!-- Submit button -->
     <div>
      <button
       type="submit"
       disabled={isLoading}
      >
       {#if isLoading}
        Creating account...
       {:else}
        Create Account
       {/if}
      </button>
     </div>
    </form>
   </div>
   
   <!-- Footer -->
   <div>
    <p>
     Already have an account? 
     <a href="/login">
      Sign in
     </a>
    </p>
   </div>
  </div>
 </div>
</div>
</RouteGuard>