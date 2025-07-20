<script lang="ts">
 import { auth } from '$lib/stores';
</script>

<div>
 <!-- Hero Section -->
 <div>
  <div>
   <h1>
    Welcome to Nexorious
   </h1>
   <p>
    Your self-hosted game collection management service. Organize, track, and discover your gaming library like never before.
   </p>
  </div>
  
  {#if auth.value.user}
   <div>
    <div>
     <div>
      <span>
       {auth.value.user.username?.charAt(0).toUpperCase()}
      </span>
     </div>
     <div>
      <p>
       Welcome back, {auth.value.user.username}!
      </p>
      <p>
       Ready to manage your game collection?
      </p>
     </div>
    </div>
    
    <!-- Quick Actions -->
    <div>
     <a
      href="/games"
     >
      <div>
       <h3>My Games</h3>
       <p>View your collection</p>
      </div>
     </a>
     
     <a
      href="/games/add"
     >
      <div>
       <h3>Add Game</h3>
       <p>Expand your library</p>
      </div>
     </a>
     
     
     <a
      href="/dashboard"
     >
      <div>
       <h3>Dashboard</h3>
       <p>View statistics</p>
      </div>
     </a>
    </div>
   </div>
  {:else}
   <!-- Not logged in -->
   <div>
    <p>
     Please log in to start managing your game collection
    </p>
    <div>
     <a href="/login">
      Login
     </a>
     <a href="/register">
      Register
     </a>
    </div>
   </div>
  {/if}
 </div>
 
 <!-- Features Section -->
 <div>
  <div>
   <h3>
    Organize Your Library
   </h3>
   <p>
    Keep track of all your games across multiple platforms and storefronts in one unified collection.
   </p>
  </div>
  
  <div>
   <h3>
    Track Progress
   </h3>
   <p>
    Monitor your gaming progress with detailed completion levels from started to dominated.
   </p>
  </div>
  
  <div>
   <h3>
    Self-Hosted Privacy
   </h3>
   <p>
    Keep your gaming data private and secure with complete control over your self-hosted instance.
   </p>
  </div>
 </div>
</div>
