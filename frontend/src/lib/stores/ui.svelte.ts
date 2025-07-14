import { browser } from '$app/environment';

export type Theme = 'light' | 'dark' | 'system';

export type NotificationType = 'success' | 'error' | 'warning' | 'info';

export interface Notification {
  id: string;
  type: NotificationType;
  title: string;
  message?: string | undefined;
  duration?: number; // in milliseconds, 0 means permanent
  actions?: {
    label: string;
    action: () => void;
  }[];
}

export interface Modal {
  id: string;
  component: string;
  props?: Record<string, any>;
  size?: 'sm' | 'md' | 'lg' | 'xl' | 'full';
  closable?: boolean;
}

export interface UIState {
  theme: Theme;
  notifications: Notification[];
  modals: Modal[];
  isLoading: boolean;
  loadingMessage?: string | undefined;
  sidebar: {
    isOpen: boolean;
    isPinned: boolean;
  };
  preferences: {
    density: 'compact' | 'comfortable' | 'spacious';
    animations: boolean;
    pageSize: number;
  };
}

const initialState: UIState = {
  theme: 'system',
  notifications: [],
  modals: [],
  isLoading: false,
  loadingMessage: undefined as string | undefined,
  sidebar: {
    isOpen: false,
    isPinned: false
  },
  preferences: {
    density: 'comfortable',
    animations: true,
    pageSize: 20
  }
};

function createUIStore() {
  let state = $state<UIState>(initialState);

  // Load preferences from localStorage on initialization
  if (browser) {
    const storedPreferences = localStorage.getItem('ui-preferences');
    if (storedPreferences) {
      try {
        const parsedPreferences = JSON.parse(storedPreferences);
        state = {
          ...state,
          theme: parsedPreferences.theme || state.theme,
          sidebar: {
            ...state.sidebar,
            isPinned: parsedPreferences.sidebarPinned || state.sidebar.isPinned
          },
          preferences: {
            ...state.preferences,
            ...parsedPreferences.preferences
          }
        };
      } catch (error) {
        console.error('Failed to parse stored UI preferences:', error);
      }
    }

    // Apply theme to document
    applyTheme(state.theme);
  }

  // Function to apply theme to document
  function applyTheme(theme: Theme) {
    if (!browser) return;

    const root = document.documentElement;
    
    if (theme === 'system') {
      // Use system preference
      const systemTheme = window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light';
      root.classList.toggle('dark', systemTheme === 'dark');
    } else {
      root.classList.toggle('dark', theme === 'dark');
    }
  }

  // Function to save preferences to localStorage
  function savePreferences() {
    if (!browser) return;

    const toSave = {
      theme: state.theme,
      sidebarPinned: state.sidebar.isPinned,
      preferences: state.preferences
    };

    localStorage.setItem('ui-preferences', JSON.stringify(toSave));
  }

  const uiStore = {
    get value() {
      return state;
    },

    // Theme management
    setTheme: (theme: Theme) => {
      state = { ...state, theme };
      applyTheme(theme);
      savePreferences();
    },

    toggleTheme: () => {
      const newTheme = state.theme === 'light' ? 'dark' : 'light';
      state = { ...state, theme: newTheme };
      applyTheme(newTheme);
      savePreferences();
    },

    // Notification management
    addNotification: (notification: Omit<Notification, 'id'>) => {
      const id = Math.random().toString(36).substring(2, 9);
      const newNotification: Notification = {
        id,
        duration: 5000, // Default 5 seconds
        ...notification
      };

      state = {
        ...state,
        notifications: [...state.notifications, newNotification]
      };

      // Auto-remove notification after duration
      if (newNotification.duration && newNotification.duration > 0) {
        setTimeout(() => {
          state = {
            ...state,
            notifications: state.notifications.filter(n => n.id !== id)
          };
        }, newNotification.duration);
      }

      return id;
    },

    removeNotification: (id: string) => {
      state = {
        ...state,
        notifications: state.notifications.filter(n => n.id !== id)
      };
    },

    clearNotifications: () => {
      state = { ...state, notifications: [] };
    },

    // Success notification shortcut
    showSuccess: (title: string, message?: string) => {
      return uiStore.addNotification({
        type: 'success',
        title,
        message,
        duration: 3000
      });
    },

    // Error notification shortcut
    showError: (title: string, message?: string) => {
      return uiStore.addNotification({
        type: 'error',
        title,
        message,
        duration: 0 // Permanent until dismissed
      });
    },

    // Warning notification shortcut
    showWarning: (title: string, message?: string) => {
      return uiStore.addNotification({
        type: 'warning',
        title,
        message,
        duration: 5000
      });
    },

    // Info notification shortcut
    showInfo: (title: string, message?: string) => {
      return uiStore.addNotification({
        type: 'info',
        title,
        message,
        duration: 4000
      });
    },

    // Modal management
    openModal: (modal: Omit<Modal, 'id'>) => {
      const id = Math.random().toString(36).substring(2, 9);
      const newModal: Modal = {
        id,
        size: 'md',
        closable: true,
        ...modal
      };

      state = {
        ...state,
        modals: [...state.modals, newModal]
      };

      return id;
    },

    closeModal: (id: string) => {
      state = {
        ...state,
        modals: state.modals.filter(m => m.id !== id)
      };
    },

    closeAllModals: () => {
      state = { ...state, modals: [] };
    },

    // Global loading state
    setLoading: (isLoading: boolean, message?: string) => {
      state = {
        ...state,
        isLoading,
        loadingMessage: message as string | undefined
      };
    },

    // Sidebar management
    toggleSidebar: () => {
      state = {
        ...state,
        sidebar: {
          ...state.sidebar,
          isOpen: !state.sidebar.isOpen
        }
      };
    },

    openSidebar: () => {
      state = {
        ...state,
        sidebar: {
          ...state.sidebar,
          isOpen: true
        }
      };
    },

    closeSidebar: () => {
      state = {
        ...state,
        sidebar: {
          ...state.sidebar,
          isOpen: false
        }
      };
    },

    toggleSidebarPin: () => {
      state = {
        ...state,
        sidebar: {
          ...state.sidebar,
          isPinned: !state.sidebar.isPinned
        }
      };
      savePreferences();
    },

    // Preferences management
    setDensity: (density: 'compact' | 'comfortable' | 'spacious') => {
      state = {
        ...state,
        preferences: {
          ...state.preferences,
          density
        }
      };
      savePreferences();
    },

    setAnimations: (animations: boolean) => {
      state = {
        ...state,
        preferences: {
          ...state.preferences,
          animations
        }
      };
      savePreferences();
    },

    setPageSize: (pageSize: number) => {
      state = {
        ...state,
        preferences: {
          ...state.preferences,
          pageSize: Math.max(5, Math.min(100, pageSize)) // Clamp between 5 and 100
        }
      };
      savePreferences();
    },

    // Listen for system theme changes
    initSystemThemeListener: () => {
      if (!browser) return;

      const mediaQuery = window.matchMedia('(prefers-color-scheme: dark)');
      
      const handleChange = () => {
        if (state.theme === 'system') {
          applyTheme('system');
        }
      };

      mediaQuery.addEventListener('change', handleChange);
      
      // Return cleanup function
      return () => mediaQuery.removeEventListener('change', handleChange);
    }
  };
  
  return uiStore;
}

export const ui = createUIStore();