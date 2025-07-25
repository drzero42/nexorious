export type NotificationType = 'success' | 'error' | 'warning' | 'info';

export interface Notification {
	id: string;
	type: NotificationType;
	message: string;
	duration?: number;
	createdAt: Date;
}

class NotificationStore {
	private notifications = $state<Notification[]>([]);
	private idCounter = 0;

	get items(): Notification[] {
		return this.notifications;
	}

	private generateId(): string {
		return `notification-${++this.idCounter}-${Date.now()}`;
	}

	private add(type: NotificationType, message: string, duration = 5000): string {
		const notification: Notification = {
			id: this.generateId(),
			type,
			message,
			duration,
			createdAt: new Date()
		};

		this.notifications.push(notification);
		
		// Keep only the last 5 notifications to prevent memory issues
		if (this.notifications.length > 5) {
			this.notifications = this.notifications.slice(-5);
		}

		return notification.id;
	}

	showSuccess(message: string, duration = 5000): string {
		return this.add('success', message, duration);
	}

	showError(message: string, duration = 8000): string {
		return this.add('error', message, duration);
	}

	showWarning(message: string, duration = 6000): string {
		return this.add('warning', message, duration);
	}

	showInfo(message: string, duration = 5000): string {
		return this.add('info', message, duration);
	}

	remove(id: string): void {
		this.notifications = this.notifications.filter(n => n.id !== id);
	}

	clear(): void {
		this.notifications = [];
	}

	// Helper method to show API error messages
	showApiError(error: unknown, defaultMessage = 'An unexpected error occurred'): string {
		let message = defaultMessage;
		
		if (error instanceof Error) {
			message = error.message;
		} else if (typeof error === 'string') {
			message = error;
		} else if (error && typeof error === 'object' && 'message' in error) {
			message = String(error.message);
		}

		return this.showError(message);
	}
}

export const notifications = new NotificationStore();