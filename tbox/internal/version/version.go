// Package version holds the tbox release version.
package version

// Version is the tariboy product version, reported by `tbox version`. tbox
// ships with every tariboy release, and scripts/set-version.sh moves this copy
// together with internal/version.Version; the root test
// TestTboxVersionMatchesCanonicalVersion fails when the two differ. The tbox
// module cannot import the tariboy package, so the value is repeated here.
const Version = "0.116.0"
