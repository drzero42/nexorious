# Known issues

## Expanded platform badges are too big
When expanding a platform badge, the text for the name of the platform takes up a lot of space. Since the name of the platform is already shown in the collapsed badge, we don't need it to show again in the expanded box. The box should only show the storefronts. These should have the same text size as the platform name in the collapsed platform badge and should not be abbreviated with ... 

## Platforms for badges are hardcoded in the frontend
The code for the frontend contains a list of platforms with their styling. This means that all platforms must both be hardcoded in the frontend and be in the backend. This is very bad architecture. Instead the styling of the platform badge should not be dependent on the platform. All badges should be styled the same. The name of the platform as well as a logo is shown, which is enough.
