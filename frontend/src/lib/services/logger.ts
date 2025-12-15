import { dev } from '$app/environment';

export type LogLevel = 'debug' | 'info' | 'warn' | 'error';

interface LoggerOptions {
  prefix?: string;
  enabled?: boolean;
}

/**
 * Logger service that only outputs in development mode.
 * Use this instead of console.log/warn/error in production code.
 */
class Logger {
  private prefix: string;
  private enabled: boolean;

  constructor(options: LoggerOptions = {}) {
    this.prefix = options.prefix || '';
    this.enabled = options.enabled ?? dev;
  }

  /**
   * Create a child logger with a specific prefix
   */
  child(prefix: string): Logger {
    const fullPrefix = this.prefix ? `${this.prefix}:${prefix}` : prefix;
    return new Logger({ prefix: fullPrefix, enabled: this.enabled });
  }

  /**
   * Debug level - verbose logging for development
   */
  debug(message: string, ...args: unknown[]): void {
    if (this.enabled) {
      console.log(this.format(message), ...args);
    }
  }

  /**
   * Info level - general information
   */
  info(message: string, ...args: unknown[]): void {
    if (this.enabled) {
      console.log(this.format(message), ...args);
    }
  }

  /**
   * Warn level - warnings that don't prevent operation
   */
  warn(message: string, ...args: unknown[]): void {
    if (this.enabled) {
      console.warn(this.format(message), ...args);
    }
  }

  /**
   * Error level - errors that should always be logged (even in production for critical issues)
   * Note: In production, consider sending these to an error tracking service
   */
  error(message: string, ...args: unknown[]): void {
    // Errors are always logged, even in production
    console.error(this.format(message), ...args);
  }

  private format(message: string): string {
    return this.prefix ? `[${this.prefix}] ${message}` : message;
  }
}

// Default logger instance
export const logger = new Logger();

// Pre-configured loggers for common modules
export const loggers = {
  platforms: new Logger({ prefix: 'PLATFORMS' }),
  auth: new Logger({ prefix: 'AUTH' }),
  api: new Logger({ prefix: 'API' }),
  ui: new Logger({ prefix: 'UI' }),
  import: new Logger({ prefix: 'IMPORT' }),
  tags: new Logger({ prefix: 'TAGS' }),
  darkadia: new Logger({ prefix: 'DARKADIA' }),
  websocket: new Logger({ prefix: 'WEBSOCKET' }),
  admin: new Logger({ prefix: 'ADMIN' }),
};

// Export Logger class for custom instances
export { Logger };
