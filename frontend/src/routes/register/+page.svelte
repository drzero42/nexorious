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
<div class="flex min-h-full flex-col justify-center py-12 sm:px-6 lg:px-8">
 <div class="sm:mx-auto sm:w-full sm:max-w-md">
  <div class="bg-white px-4 py-8 shadow sm:rounded-lg sm:px-10">
   <!-- Header -->
   <div class="sm:mx-auto sm:w-full sm:max-w-md mb-8">
    <div class="text-center">
     <h1 class="text-3xl font-bold tracking-tight text-gray-900">Nexorious</h1>
     <p class="text-sm text-gray-600">Game Collection Manager</p>
    </div>
   </div>
   
   <div>
    <!-- Form Header -->
    <div class="mb-6">
     <h2 class="text-center text-2xl font-bold leading-9 tracking-tight text-gray-900">
      Create Account
     </h2>
     <p class="mt-2 text-center text-sm text-gray-600">
      Join Nexorious to start managing your game collection
     </p>
    </div>

    {#if error}
     <div class="mb-4 rounded-md bg-red-50 p-4">
      <div class="flex">
       <div class="flex-shrink-0">
        <svg class="h-5 w-5 text-red-400" viewBox="0 0 20 20" fill="currentColor">
         <path fill-rule="evenodd" d="M10 18a8 8 0 100-16 8 8 0 000 16zM8.28 7.22a.75.75 0 00-1.06 1.06L8.94 10l-1.72 1.72a.75.75 0 101.06 1.06L10 11.06l1.72 1.72a.75.75 0 101.06-1.06L11.06 10l1.72-1.72a.75.75 0 00-1.06-1.06L10 8.94 8.28 7.22z" clip-rule="evenodd" />
        </svg>
       </div>
       <div class="ml-3">
        <p class="text-sm text-red-800">
         {error}
        </p>
       </div>
      </div>
     </div>
    {/if}

    <form on:submit|preventDefault={handleRegister} class="space-y-6">
     <!-- Email field -->
     <div>
      <label for="email" class="form-label">
       Email Address
      </label>
      <div class="relative">
       <input
        id="email"
        type="email"
        bind:value={email}
        on:keydown={handleKeydown}
        required
        placeholder="Enter your email address"
        class="form-input pr-10"
        disabled={isLoading}
       />
       {#if emailValid}
        <div class="absolute inset-y-0 right-0 flex items-center pr-3">
         <div class="text-green-500">
          <svg class="h-5 w-5" fill="currentColor" viewBox="0 0 20 20">
           <path fill-rule="evenodd" d="M16.707 5.293a1 1 0 010 1.414l-8 8a1 1 0 01-1.414 0l-4-4a1 1 0 011.414-1.414L8 12.586l7.293-7.293a1 1 0 011.414 0z" clip-rule="evenodd" />
          </svg>
         </div>
        </div>
       {/if}
      </div>
     </div>

     <!-- Username field -->
     <div>
      <label for="username" class="form-label">
       Username
      </label>
      <div class="relative">
       <input
        id="username"
        type="text"
        bind:value={username}
        on:keydown={handleKeydown}
        required
        placeholder="Choose a unique username"
        class="form-input pr-10"
        disabled={isLoading}
       />
       {#if usernameChecking}
        <div class="absolute inset-y-0 right-0 flex items-center pr-3">
         <div class="text-gray-400">
          <svg class="animate-spin h-5 w-5" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24">
           <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
           <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
          </svg>
         </div>
        </div>
       {:else if username.length >= 3 && usernameAvailable}
        <div class="absolute inset-y-0 right-0 flex items-center pr-3">
         <div class="text-green-500">
          <svg class="h-5 w-5" fill="currentColor" viewBox="0 0 20 20">
           <path fill-rule="evenodd" d="M16.707 5.293a1 1 0 010 1.414l-8 8a1 1 0 01-1.414 0l-4-4a1 1 0 011.414-1.414L8 12.586l7.293-7.293a1 1 0 011.414 0z" clip-rule="evenodd" />
          </svg>
         </div>
        </div>
       {:else if username.length >= 3 && !usernameAvailable}
        <div class="absolute inset-y-0 right-0 flex items-center pr-3">
         <div class="text-red-500">
          <svg class="h-5 w-5" fill="currentColor" viewBox="0 0 20 20">
           <path fill-rule="evenodd" d="M4.293 4.293a1 1 0 011.414 0L10 8.586l4.293-4.293a1 1 0 111.414 1.414L11.414 10l4.293 4.293a1 1 0 01-1.414 1.414L10 11.414l-4.293 4.293a1 1 0 01-1.414-1.414L8.586 10 4.293 5.707a1 1 0 010-1.414z" clip-rule="evenodd" />
          </svg>
         </div>
        </div>
       {/if}
      </div>
     </div>

     <!-- Password field -->
     <div>
      <label for="password" class="form-label">
       Password
      </label>
      <input
       id="password"
       type="password"
       bind:value={password}
       on:keydown={handleKeydown}
       required
       placeholder="Enter a strong password"
       class="form-input"
       disabled={isLoading}
      />
      <!-- Password strength meter -->
      {#if password}
       <div class="mt-2">
        <div class="flex justify-between items-center mb-1">
         <span class="text-xs text-gray-500">Password strength</span>
         <span class="text-xs text-gray-500">
          {#if passwordStrength < 2}
           Weak
          {:else if passwordStrength < 4}
           Medium
          {:else}
           Strong
          {/if}
         </span>
        </div>
        <div class="w-full bg-gray-200 rounded-full h-2">
         <div 
          class="h-2 rounded-full transition-all duration-300 {passwordStrength < 2 ? 'bg-red-500' : passwordStrength < 4 ? 'bg-yellow-500' : 'bg-green-500'}"
          style="width: {(passwordStrength / 5) * 100}%"
         ></div>
        </div>
        <p class="mt-1 text-xs text-gray-500">
         {#if passwordStrength < 3}
          Add numbers, symbols, and uppercase letters
         {:else if passwordStrength < 5}
          Add more characters for better security
         {:else}
          Excellent password strength
         {/if}
        </p>
       </div>
      {/if}
     </div>

     <!-- Confirm Password field -->
     <div>
      <label for="confirmPassword" class="form-label">
       Confirm Password
      </label>
      <div class="relative">
       <input
        id="confirmPassword"
        type="password"
        bind:value={confirmPassword}
        on:keydown={handleKeydown}
        required
        placeholder="Confirm your password"
        class="form-input {confirmPassword && password !== confirmPassword ? 'border-red-300 text-red-900 focus:border-red-500 focus:ring-red-500' : ''}"
        disabled={isLoading}
       />
       {#if confirmPassword && password === confirmPassword}
        <div class="absolute inset-y-0 right-0 flex items-center pr-3">
         <div class="text-green-500">
          <svg class="h-5 w-5" fill="currentColor" viewBox="0 0 20 20">
           <path fill-rule="evenodd" d="M16.707 5.293a1 1 0 010 1.414l-8 8a1 1 0 01-1.414 0l-4-4a1 1 0 011.414-1.414L8 12.586l7.293-7.293a1 1 0 011.414 0z" clip-rule="evenodd" />
          </svg>
         </div>
        </div>
       {/if}
      </div>
      {#if confirmPassword && password !== confirmPassword}
       <p class="mt-1 text-sm text-red-600">Passwords do not match</p>
      {/if}
     </div>

     <!-- Submit button -->
     <div>
      <button
       type="submit"
       disabled={isLoading}
       class="flex w-full justify-center rounded-md bg-primary-500 px-3 py-2 text-sm font-semibold text-white shadow-sm hover:bg-primary-600 focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-primary-500 disabled:opacity-50 disabled:cursor-not-allowed"
      >
       {#if isLoading}
        <svg class="animate-spin -ml-1 mr-2 h-4 w-4 text-white" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24">
         <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
         <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
        </svg>
        Creating account...
       {:else}
        Create Account
       {/if}
      </button>
     </div>
    </form>
   </div>
   
   <!-- Footer -->
   <div class="mt-8 text-center">
    <p class="text-sm text-gray-600">
     Already have an account? 
     <a href="/login" class="font-medium text-primary-600 hover:text-primary-500">
      Sign in
     </a>
    </p>
   </div>
  </div>
 </div>
</div>
</RouteGuard>