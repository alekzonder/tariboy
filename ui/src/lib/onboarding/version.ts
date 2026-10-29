// scripts/set-version.sh writes this file at release time, so the UI knows
// which release it belongs to without asking a daemon.
import releaseVersion from "../../../../scripts/release-version.txt?raw";

export const UI_VERSION = releaseVersion.trim();
