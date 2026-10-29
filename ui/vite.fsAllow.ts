import path from "node:path";

// The onboarding imports CHANGELOG.md and scripts/release-version.txt from the
// repository root with `?raw`. Vite matches a `?raw` id only below an allowed
// directory, so the dev server may read the repository root; its default deny
// list (.env, certificates) still applies.
export const fsAllow = [path.resolve(__dirname, "..")];
