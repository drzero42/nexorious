import { ExternalLink } from 'lucide-react';

import { useVersion } from '@/hooks';
import { GITHUB_REPO_URL } from '@/lib/repo';
import { isValidRelease } from '@/lib/version-compare';
import { WhatsNew } from './whats-new';

export function VersionFooter() {
  const { data: versionInfo } = useVersion();

  if (!versionInfo?.version) return null;

  return (
    <div className="px-4 pb-3 text-xs text-muted-foreground">
      <div>Version: {versionInfo.version}</div>
      <div className="flex items-center gap-3">
        <a
          href={GITHUB_REPO_URL}
          target="_blank"
          rel="noopener noreferrer"
          className="inline-flex items-center gap-1 underline hover:text-foreground"
        >
          GitHub
          <ExternalLink className="h-3 w-3" />
        </a>
        {isValidRelease(versionInfo.version) && <WhatsNew />}
      </div>
      {versionInfo.update_available && versionInfo.release_url && (
        <a
          href={versionInfo.release_url}
          target="_blank"
          rel="noopener noreferrer"
          className="underline hover:text-foreground"
        >
          A newer version is available
        </a>
      )}
    </div>
  );
}
