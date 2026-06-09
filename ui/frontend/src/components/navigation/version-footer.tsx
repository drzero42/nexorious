import { useVersion } from '@/hooks';

export function VersionFooter() {
  const { data: versionInfo } = useVersion();

  if (!versionInfo?.version) return null;

  return (
    <div className="px-4 pb-3 text-xs text-muted-foreground">
      <div>Version: {versionInfo.version}</div>
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
