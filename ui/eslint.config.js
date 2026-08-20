import js from '@eslint/js'
import globals from 'globals'
import reactHooks from 'eslint-plugin-react-hooks'
import reactRefresh from 'eslint-plugin-react-refresh'
import tseslint from 'typescript-eslint'
import { defineConfig, globalIgnores } from 'eslint/config'

export default defineConfig([
  globalIgnores(['dist']),
  {
    files: ['**/*.{ts,tsx}'],
    extends: [
      js.configs.recommended,
      tseslint.configs.recommended,
      reactHooks.configs.flat.recommended,
      reactRefresh.configs.vite,
    ],
    languageOptions: {
      globals: globals.browser,
    },
  },
  // These pre-existing files have not yet been migrated to React 19's stricter
  // hook checks. Keep the compatibility boundary explicit so new UI (including
  // the Judge views) continues to receive the full rule set.
  {
    files: [
      'src/components/AgentFormFields.tsx',
      'src/components/AgentSubscriptions.tsx',
      'src/components/ComboField.tsx',
      'src/components/DaemonProvider.tsx',
      'src/components/ImageLayout.tsx',
      'src/components/PathAutocomplete.tsx',
      'src/components/ui/badge.tsx',
      'src/components/ui/button.tsx',
    ],
    rules: {
      'react-refresh/only-export-components': 'off',
    },
  },
  {
    files: [
      'src/components/AgentLayout.tsx',
      'src/components/FullAuditLog.tsx',
      'src/hooks/usePolling.ts',
    ],
    rules: {
      'react-hooks/refs': 'off',
    },
  },
  {
    files: [
      'src/components/FullAuditLog.tsx',
      'src/components/ImageLayout.tsx',
      'src/components/IterationAuditLog.tsx',
      'src/pages/AuditLogPage.tsx',
      'src/pages/ChannelsPage.tsx',
      'src/pages/ImagePrompt.tsx',
      'store/App.tsx',
    ],
    rules: {
      'react-hooks/set-state-in-effect': 'off',
    },
  },
  {
    files: ['src/components/InboxComposer.test.tsx'],
    rules: {
      '@typescript-eslint/no-explicit-any': 'off',
    },
  },
])
