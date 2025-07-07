# Nexorious Frontend

SvelteKit frontend for the Nexorious Game Collection Management Service.

## Setup

1. Install dependencies:
```bash
npm install
```

2. Start the development server:
```bash
npm run dev

# or start the server and open the app in a new browser tab
npm run dev -- --open
```

## Building

To create a production version of your app:

```bash
npm run build
```

You can preview the production build with `npm run preview`.

## Features

- **SvelteKit**: Modern web framework with TypeScript support
- **Tailwind CSS**: Utility-first CSS framework with dark mode support
- **PWA Support**: Progressive Web App with offline capabilities
- **Authentication**: JWT-based authentication with refresh tokens
- **Responsive Design**: Mobile-first responsive layout

## Configuration

The frontend expects the backend API to be available at `http://localhost:8000` by default. You can configure this in the authentication store or add environment variables as needed.

## Development

- `npm run dev` - Start development server
- `npm run build` - Build for production
- `npm run preview` - Preview production build
- `npm run lint` - Run linter (if configured)
- `npm run format` - Format code (if configured)
