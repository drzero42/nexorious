// PWA functionality for service worker management and installation prompts

export function initializePWA(): void {
  // Initialize PWA features
  if ('serviceWorker' in navigator) {
    navigator.serviceWorker.register('/service-worker.js')
      .then(registration => {
        console.log('Service Worker registered:', registration);
      })
      .catch(error => {
        console.log('Service Worker registration failed:', error);
      });
  }
}

export function initializeInstallPrompt(): void {
  // Initialize PWA install prompt handling
  window.addEventListener('beforeinstallprompt', (e) => {
    e.preventDefault();
    // Store the event for later use (could be accessed via a store or global)
    console.log('PWA install prompt ready', e);
  });
}