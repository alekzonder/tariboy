package loop

import (
	"sort"
	"strings"
)

// secretPlaceholder is the fixed redaction shown in audit records in place of a
// masked secret value. Key names are always kept visible; only values are hidden.
const secretPlaceholder = "***"

// minSubstringSecretLen is the shortest secret value we redact as a raw
// substring (rule (b)). Very short values ('1', 'true', 'prod', 'dev') are
// common substrings of unrelated argv/env entries, so substring-masking them
// would nuke the whole record and make the audit unreadable. Short values are
// still fully masked when their KEY matches a secret key (rule (a)); only the
// blind substring scan skips them.
const minSubstringSecretLen = 6

// maskLaunch returns log- and audit-safe copies of the harness launch argv and
// env with secret material redacted. It never touches the argv/env actually
// handed to the spawned child, so masking here cannot change how the harness
// runs.
//
// secrets is the per-agent SecretMap (arbitrary user-defined key -> value).
// extraValues holds additional secret strings that have no env key of their own
// — e.g. the minted proxy token, which reaches the child only embedded inside
// ANTHROPIC_BASE_URL / OPENAI_BASE_URL.
//
// Masking rules (per task spec):
//
//	(a) any env entry whose KEY equals a known provider credential key or a
//	    known non-empty secret key -> its VALUE becomes the placeholder.
//	(b) any occurrence of a secret value or an extra value at least
//	    minSubstringSecretLen bytes long, appearing as a SUBSTRING inside an env
//	    VALUE or an argv entry, is replaced (covers secrets embedded in URLs).
//	    KEY names stay visible.
//
// Empty secret values are skipped so an empty string does not mask everything;
// values shorter than minSubstringSecretLen are skipped for rule (b) so a common
// short value does not over-redact the whole record (they still hit rule (a)).
func maskLaunch(argv, env []string, secrets map[string]string, extraValues ...string) (maskedArgv, maskedEnv []string) {
	// Collect secret VALUES long enough to redact as substrings.
	values := make([]string, 0, len(secrets)+len(extraValues))
	for _, v := range secrets {
		if len(v) >= minSubstringSecretLen {
			values = append(values, v)
		}
	}
	for _, v := range extraValues {
		if len(v) >= minSubstringSecretLen {
			values = append(values, v)
		}
	}
	// Longest-first: a value that contains another is masked before its
	// substring, so no partial leftover survives.
	sort.Slice(values, func(i, j int) bool { return len(values[i]) > len(values[j]) })

	redactSubstrings := func(s string) string {
		for _, v := range values {
			if strings.Contains(s, v) {
				s = strings.ReplaceAll(s, v, secretPlaceholder)
			}
		}
		return s
	}

	maskedArgv = make([]string, len(argv))
	for i, a := range argv {
		maskedArgv[i] = redactSubstrings(a)
	}

	maskedEnv = make([]string, len(env))
	for i, kv := range env {
		idx := strings.Index(kv, "=")
		if idx < 0 {
			// Bare entry with no '=' (just a key name): preserve verbatim. There
			// is no value to hide, and appending '=' would corrupt the record.
			maskedEnv[i] = kv
			continue
		}
		key, val := kv[:idx], kv[idx+1:]
		// Credential-shaped keys may be inherited from the daemon environment
		// rather than the per-agent SecretMap — the daemon carries the account's
		// whole login-shell environment, so anything an rc file exports arrives
		// here. They remain available to the child, but their values must never
		// enter logs or audit records.
		if val != "" && isCredentialShapedKey(key) {
			maskedEnv[i] = key + "=" + secretPlaceholder
			continue
		}
		// Rule (a): key equals a known non-empty secret key -> mask whole value.
		if sv, ok := secrets[key]; ok && sv != "" {
			maskedEnv[i] = key + "=" + secretPlaceholder
			continue
		}
		// Rule (b): mask secret values embedded anywhere in the value.
		maskedEnv[i] = key + "=" + redactSubstrings(val)
	}
	return maskedArgv, maskedEnv
}

// isCredentialShapedKey reports whether a variable NAME alone is enough reason to
// hide its value from an audit record. Matching on the name is deliberate: an
// inherited credential has no entry in the per-agent SecretMap, so there is no
// value to match against, and the name is all that is left. Over-redaction here
// costs an unreadable field in one audit record; under-redaction writes a live
// credential to disk, so the list errs wide.
func isCredentialShapedKey(key string) bool {
	upper := strings.ToUpper(key)
	for _, marker := range []string{
		"TOKEN", "SECRET", "PASSWORD", "PASSWD", "CREDENTIAL", "APIKEY", "AUTH",
	} {
		if strings.Contains(upper, marker) {
			return true
		}
	}
	return strings.HasSuffix(upper, "_KEY") || upper == "KEY"
}
