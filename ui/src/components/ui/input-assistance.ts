/**
 * Turns off spell checking, autocorrection and autocapitalization on every
 * shared text control. Names, paths and identifiers must stay as typed, and
 * WebKit would otherwise capitalize or replace them. Spread it before the
 * caller's props so one field can opt back in, e.g. `autoCorrect="on"`.
 */
export const TEXT_INPUT_ASSISTANCE = {
  spellCheck: false,
  autoCorrect: "off",
  autoCapitalize: "off",
} as const;

/** The same defaults as DOM attributes, for editors that render outside React. */
export const TEXT_INPUT_ASSISTANCE_ATTRIBUTES = {
  spellcheck: "false",
  autocorrect: "off",
  autocapitalize: "off",
} as const;
