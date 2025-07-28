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

## Testing

The frontend uses Vitest for testing with comprehensive coverage reporting.

### Running Tests

- `npm test` - Run tests in watch mode (interactive)
- `npm run test:ui` - Run tests with Vitest UI interface
- `npm run test:run` - Run all tests once (CI mode)
- `npm run test:coverage` - Run tests with coverage report

### Test Coverage

The project maintains a **70% coverage threshold** for:
- Lines of code
- Functions
- Branches
- Statements

#### Coverage Reports

When running `npm run test:coverage`, three types of reports are generated:

1. **Text Report** - Displayed in terminal with summary statistics
2. **JSON Report** - Machine-readable coverage data
3. **HTML Report** - Detailed interactive coverage report in `coverage/` directory

To view the detailed HTML coverage report:
```bash
npm run test:coverage
# Open coverage/index.html in your browser
```

#### Coverage Configuration

The following files are excluded from coverage analysis:
- `node_modules/`
- `src/test-utils/` - Test utilities and mocks
- `**/*.d.ts` - TypeScript declaration files
- `**/*.config.*` - Configuration files
- `src/app.html` - HTML template file

Tests must maintain the 70% coverage threshold to pass. You can view current coverage status by running the coverage command and checking the terminal output or HTML report.
