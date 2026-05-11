import { defineConfig, globalIgnores } from 'eslint/config';
import js from '@eslint/js';
import tseslint from 'typescript-eslint';
import reactHooks from 'eslint-plugin-react-hooks';
import reactRefresh from 'eslint-plugin-react-refresh';

export default defineConfig([
  globalIgnores(['dist/**', 'coverage/**']),
  js.configs.recommended,
  ...tseslint.configs.recommended,
  {
    plugins: {
      'react-hooks': reactHooks,
      'react-refresh': reactRefresh,
    },
    rules: {
      ...reactHooks.configs.recommended.rules,
      'react-refresh/only-export-components': ['warn', { allowConstantExport: true }],
    },
  },
  // TanStack Router requires exporting `Route` alongside the component in route files
  {
    files: ['src/routes/**/*.tsx', 'src/routes/**/*.ts'],
    rules: {
      'react-refresh/only-export-components': 'off',
    },
  },
  // shadcn/ui components export variants (e.g. buttonVariants) alongside components
  {
    files: ['src/components/ui/**/*.tsx'],
    rules: {
      'react-refresh/only-export-components': 'off',
    },
  },
  // Test utilities are not production components
  {
    files: ['src/test/**/*.tsx', 'src/test/**/*.ts'],
    rules: {
      'react-refresh/only-export-components': 'off',
    },
  },
]);
