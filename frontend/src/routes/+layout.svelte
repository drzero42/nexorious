<script lang="ts">
  import '../app.css';
  import { auth } from '$lib/stores';
  import { onMount } from 'svelte';
  
  let mobileMenuOpen = false;
  
  onMount(() => {
    // Check if user is authenticated and refresh token if needed
    const authState = auth.value;
    if (authState.accessToken && authState.refreshToken) {
      // Optionally refresh token on app start
      auth.refreshAuth();
    }
    
    
  });
  
  function closeMobileMenu() {
    mobileMenuOpen = false;
  }
</script>

<svelte:head>
  <title>Nexorious Game Collection</title>
  <meta name="description" content="Self-hostable game collection management" />
  <meta name="viewport" content="width=device-width, initial-scale=1" />
</svelte:head>

<div>
  <header>
    <div>
      <div>
        <!-- Logo/Brand -->
        <div>
          <a href="/">
            <h1>
              Nexorious
            </h1>
          </a>
        </div>
        
        <!-- Desktop Navigation -->
        <nav>
          {#if auth.value.user}
            <a href="/games">
              My Games
            </a>
            <a href="/dashboard">
              Dashboard
            </a>
          {/if}
        </nav>

        <!-- Right Side Actions -->
        <div>
          
          {#if auth.value.user}
            <div>
              <div>
                <div>
                  <span>
                    {auth.value.user.username?.charAt(0).toUpperCase()}
                  </span>
                </div>
                <span>
                  {auth.value.user.username}
                </span>
              </div>
              <button on:click={() => auth.logout()}>
                Logout
              </button>
            </div>
            
            <!-- Mobile menu button -->
            <button
              on:click={() => mobileMenuOpen = !mobileMenuOpen}
              aria-label="Toggle mobile menu"
            >
              {#if mobileMenuOpen}
                ✕
              {:else}
                ☰
              {/if}
            </button>
          {:else}
            <a href="/login">
              Login
            </a>
          {/if}
        </div>
      </div>
    </div>
  </header>

  <!-- Mobile menu -->
  {#if mobileMenuOpen && auth.value.user}
    <div>
      <div>
        <a
          href="/games"
          on:click={closeMobileMenu}
        >
          My Games
        </a>
        <a
          href="/dashboard"
          on:click={closeMobileMenu}
        >
          Dashboard
        </a>
      </div>
      <div>
        <div>
          <div>
            <div>
              <span>
                {auth.value.user.username?.charAt(0).toUpperCase()}
              </span>
            </div>
          </div>
          <div>
            <div>
              {auth.value.user.username}
            </div>
            <div>
              {auth.value.user.email}
            </div>
          </div>
        </div>
        <div>
          <button
            on:click={() => { auth.logout(); closeMobileMenu(); }}
          >
            Sign out
          </button>
        </div>
      </div>
    </div>
  {/if}

  <main>
    <div>
      <slot />
    </div>
  </main>
</div>


