import { browser } from '$app/environment';
import { Workbox } from 'workbox-window';

let wb: Workbox | undefined;

export function initializePWA() {
	if (!browser) return;

	if ('serviceWorker' in navigator) {
		wb = new Workbox('/service-worker.js');

		// Add event listeners to handle service worker events
		wb.addEventListener('installed', (event) => {
			console.log('Service Worker installed');
			if (event.isUpdate) {
				// Show update available notification
				showUpdateAvailable();
			}
		});

		wb.addEventListener('waiting', (event) => {
			console.log('Service Worker waiting');
			showUpdateAvailable();
		});

		wb.addEventListener('controlling', (event) => {
			console.log('Service Worker controlling');
			// Refresh the page to load updated content
			window.location.reload();
		});

		wb.addEventListener('activated', (event) => {
			console.log('Service Worker activated');
		});

		// Register the service worker
		wb.register();
	}
}

function showUpdateAvailable() {
	// Dispatch a custom event that components can listen to
	const event = new CustomEvent('pwa-update-available', {
		detail: { wb }
	});
	window.dispatchEvent(event);
}

export function updateServiceWorker() {
	if (wb) {
		wb.messageSkipWaiting();
	}
}

// Install prompt functionality
let deferredPrompt: any;

export function initializeInstallPrompt() {
	if (!browser) return;

	window.addEventListener('beforeinstallprompt', (e) => {
		// Prevent the mini-infobar from appearing on mobile
		e.preventDefault();
		// Save the event so it can be triggered later
		deferredPrompt = e;
		
		// Dispatch custom event to show install button
		const event = new CustomEvent('pwa-install-available');
		window.dispatchEvent(event);
	});

	window.addEventListener('appinstalled', () => {
		console.log('PWA installed');
		deferredPrompt = null;
		
		// Dispatch custom event to hide install button
		const event = new CustomEvent('pwa-installed');
		window.dispatchEvent(event);
	});
}

export async function showInstallPrompt() {
	if (!deferredPrompt) return false;

	// Show the install prompt
	deferredPrompt.prompt();
	
	// Wait for the user to respond to the prompt
	const { outcome } = await deferredPrompt.userChoice;
	
	console.log(`User response to install prompt: ${outcome}`);
	
	// Clear the deferred prompt
	deferredPrompt = null;
	
	return outcome === 'accepted';
}

export function isPWAInstalled(): boolean {
	if (!browser) return false;
	
	return (
		window.matchMedia('(display-mode: standalone)').matches ||
		window.matchMedia('(display-mode: fullscreen)').matches ||
		// @ts-expect-error - navigator.standalone is iOS specific
		navigator.standalone === true
	);
}

export function isOnline(): boolean {
	if (!browser) return true;
	return navigator.onLine;
}