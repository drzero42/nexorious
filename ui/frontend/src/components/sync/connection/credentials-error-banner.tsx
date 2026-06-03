import { AlertTriangle } from 'lucide-react';

interface CredentialsErrorBannerProps {
  title: string;
  description: string;
}

/** Yellow re-authorization banner shown when stored credentials are invalid. */
export function CredentialsErrorBanner({ title, description }: CredentialsErrorBannerProps) {
  return (
    <div className="flex items-start gap-3 rounded-lg border border-yellow-200 bg-yellow-50 p-4 dark:border-yellow-800 dark:bg-yellow-900/20">
      <AlertTriangle className="h-5 w-5 text-yellow-600 dark:text-yellow-400" />
      <div>
        <p className="font-medium text-yellow-800 dark:text-yellow-200">{title}</p>
        <p className="text-sm text-yellow-700 dark:text-yellow-300">{description}</p>
      </div>
    </div>
  );
}
