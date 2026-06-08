interface StorefrontLabelProps {
  displayName: string;
  storeUrl?: string;
}

export function StorefrontLabel({ displayName, storeUrl }: StorefrontLabelProps) {
  if (storeUrl) {
    return (
      <a
        href={storeUrl}
        target="_blank"
        rel="noopener noreferrer"
        className="text-sm text-muted-foreground underline-offset-2 hover:underline"
      >
        ({displayName})
      </a>
    );
  }
  return <span className="text-sm text-muted-foreground">({displayName})</span>;
}
