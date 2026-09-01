GO      ?= go
BINDIR  := bin
PREFIX  ?= $(HOME)/.local
INSTALLDIR := $(PREFIX)/bin
BINARIES := tariboyd tariboy tariboy-shim tariboy-store tariboy-plugin-telegram
SA := $(BINDIR)/tariboy
# Desktop E2E specs receive separate owner-only state from their fixture, so
# two WebDriver workers can run safely on a normal development host. Keep this
# bounded because every worker owns a full WebKit, Tauri, daemon, and Xvfb
# stack. Callers with a constrained host can override this to 1.
DESKTOP_E2E_WORKERS ?= 2
# A standalone package build owns its UI bootstrap. Verification entry points
# override this to 0 so their documented read-only dependency contract remains
# intact in a shared working tree.
DESKTOP_INSTALL_UI_DEPS ?= 1

# `go list ./...` descends into a Go package shipped by one UI dependency.
# Test only packages owned by this module; node_modules is prepared tooling, not
# Go source covered by Tariboy's quality gates. Keep enumeration in the three
# recipes that need it, so a failed `go list` is never hidden by the filter.

export CGO_ENABLED=0

.PHONY: build build-basic-image install uninstall setup check full-check test smoke-contract-test smoke-image-skills-contract fmt fmt-check vet e2e workflow-e2e iteration-timeout-e2e group-request-deadline-e2e smoke full-smoke ui store-ui docs clean up start down attach a desktop desktop-alpha desktop-binaries desktop-version-check desktop-lock-check desktop-platform-check desktop-tools-check desktop-preflight desktop-smoke desktop-e2e-tools-check desktop-e2e-build desktop-e2e server-install

build-basic-image:
	$(GO) run ./internal/builtinimages/generate -source internal/builtinimages/source -output internal/builtinimages/generated -version $(VERSION)

build: build-basic-image
	$(GO) build -trimpath -o $(BINDIR)/tariboyd ./cmd/tariboyd
	$(GO) build -trimpath -o $(BINDIR)/tariboy ./cmd/tariboy
	$(GO) build -trimpath -o $(BINDIR)/tariboy-shim ./cmd/tariboy-shim
	$(GO) build -trimpath -o $(BINDIR)/tariboy-store ./cmd/tariboy-store
	$(GO) build -trimpath -o $(BINDIR)/tariboy-plugin-telegram ./cmd/tariboy-plugin-telegram

install:
	$(MAKE) build
	mkdir -p $(INSTALLDIR)
	@for b in $(BINARIES); do \
		echo "install $(BINDIR)/$$b -> $(INSTALLDIR)/$$b"; \
		install -m 0755 $(BINDIR)/$$b $(INSTALLDIR)/$$b; \
	done

server-install: build
	@set -eu; \
	root="$(HOME)/.local/lib/tariboy"; \
	mkdir -p "$$root"; \
	stage=$$(mktemp -d "$$root/.stage-make-XXXXXXXX"); \
	trap 'rm -rf -- "$$stage"' EXIT HUP INT TERM; \
	for b in $(BINARIES); do install -m 0755 "$(BINDIR)/$$b" "$$stage/$$b"; done; \
	printf '%s\n' "$(VERSION)" >"$$stage/VERSION"; \
	cp desktop/src-tauri/src/remote-install.sh "$$stage/remote-install.sh"; \
	(cd "$$stage" && sha256sum $(BINARIES) >SHA256SUMS); \
	HOME="$(HOME)" sh "$$stage/remote-install.sh" "$(VERSION)" "$${stage##*/}"; \
	for b in $(BINARIES); do \
		test "$$($(HOME)/.local/bin/$$b --version)" = "$(VERSION)"; \
	done; \
	echo "server binaries installed: $(VERSION)"

setup:
	./scripts/setup.sh

uninstall:
	@for b in $(BINARIES); do \
		echo "rm -f $(INSTALLDIR)/$$b"; \
		rm -f $(INSTALLDIR)/$$b; \
	done

test:
	@packages=$$($(GO) list ./...) || exit $$?; \
	packages=$$(printf '%s\n' "$$packages" | sed '/\/node_modules\//d'); \
	$(GO) test $$packages

smoke-contract-test:
	./scripts/tariboy-smoke-contract-test.sh
	./scripts/tariboy-branding-contract-test.sh
	./scripts/make-clean-contract-test.sh
	./scripts/server-install-contract-test.sh
	./scripts/publish-docs-contract-test.sh

smoke-image-skills-contract: build
	./scripts/image-skills-harness-smoke-contract-test.sh

fmt:
	$(GO) fmt ./...

# Report gofmt drift without rewriting anything. `fmt` above rewrites, which
# makes it unsafe to run mid-task: it silently reformats files unrelated to the
# change in hand. This target only reads. gofmt is resolved from the toolchain
# because the bare binary is not guaranteed to be on PATH, and gofmt -l exits 0
# even when it names files, so drift is read from its OUTPUT. Its STATUS is read
# as well, and first: a nonzero status means the scan did not complete, and with
# an empty stdout that state is otherwise indistinguishable from a clean tree.
# The package list is captured and guarded first because an empty expansion would
# otherwise leave gofmt with no file arguments, reading stdin and reporting a
# clean tree it never scanned.
fmt-check:
	@dirs=$$($(GO) list -f '{{.Dir}}' ./...) || exit $$?; \
	dirs=$$(printf '%s\n' "$$dirs" | sed '/\/node_modules\//d'); \
	if [ -z "$$dirs" ]; then \
		echo "gofmt check did not run, the package list is empty and nothing was scanned:"; \
		echo "the filtered $(GO) package list produced no directories"; \
		echo "probable cause: an unresolvable Go toolchain, a GOFLAGS/GOWORK misconfiguration, broken module state, or a working directory outside the module"; \
		exit 1; \
	fi; \
	files=$$($$($(GO) env GOROOT)/bin/gofmt -l $$dirs); \
	status=$$?; \
	if [ "$$status" -ne 0 ]; then \
		echo "gofmt check did not run, gofmt exited $$status and the scan is incomplete:"; \
		echo "probable cause: an unresolvable gofmt binary, an unreadable package directory, or a directory path that word-split into arguments that do not exist"; \
		if [ -n "$$files" ]; then \
			echo "gofmt named these files before it failed:"; \
			echo "$$files"; \
		else \
			echo "gofmt named no files; see its stderr above"; \
		fi; \
		exit 1; \
	fi; \
	if [ -n "$$files" ]; then \
		echo "gofmt drift, these files are not formatted:"; \
		echo "$$files"; \
		echo "run: gofmt -w <the files above>"; \
		exit 1; \
	fi; \
	echo "gofmt clean"

vet:
	@packages=$$($(GO) list ./...) || exit $$?; \
	packages=$$(printf '%s\n' "$$packages" | sed '/\/node_modules\//d'); \
	$(GO) vet $$packages

e2e: build
	./scripts/e2e.sh

workflow-e2e: build
	./scripts/workflow-e2e.sh

# Isolated recovery e2e for extending a running iteration timeout: repeated
# extensions, daemon adoption, persisted deadlines, and final enforcement.
# Uses its own base/runtime directories and a non-default loopback web port.
iteration-timeout-e2e: build
	./scripts/iteration-timeout-e2e.sh

# Isolated e2e for the request primitive behind the Messages skill group request (EPIC R
# R3, spec §4.2): a group request with a --deadline that gets no reply must fire
# a type=timeout event into the lead's inbox. Wall-clock (scheduler ticks once a
# second). Never touches the live daemon.
group-request-deadline-e2e: build
	./scripts/group-request-deadline-e2e.sh

smoke:
	./scripts/tariboy-smoke.sh

full-smoke:
	TARIBOY_SMOKE_FULL=1 TARIBOY_SMOKE_REAL_HARNESS=codex TARIBOY_SMOKE_REQUIRE_REAL=1 ./scripts/tariboy-smoke.sh

# Build the SPA into desktop/dist for the Tauri bundle. NOT committed — see
# `make desktop`, which always runs this before packaging.
ui:
	@test -d ui/node_modules || { echo "ui/node_modules is missing, run: cd ui && npm ci" >&2; exit 1; }
	cd ui && npm run build:desktop

store-ui:
	cd ui && npm ci && npm run build:store

docs:
	./scripts/publish-docs.sh

clean:
	git clean -dfx

.logs:
	mkdir -p .logs

start: build .logs
	$(SA) daemon start

up: build .logs
	$(SA) daemon start
	$(SA) daemon logs -f

down:
	$(SA) daemon stop

attach:
	$(SA) daemon logs -f

a: attach

# ---- Native desktop app (Tauri) ----

DESKTOP     := desktop
TAURI_DIR   := $(DESKTOP)/src-tauri
DESKTOP_BIN := $(TAURI_DIR)/resources/bin
override DESKTOP_DARWIN_BIN := desktop/src-tauri/resources/bin/darwin-arm64
override DESKTOP_LINUX_BIN := desktop/src-tauri/resources/bin/linux-x86_64
# The single source of truth for the version, read straight out of the Go const.
VERSION     := $(shell sed -n 's/^const Version = "\(.*\)"$$/\1/p' internal/version/version.go)
RELEASE_VERSION := $(shell cat scripts/release-version.txt 2>/dev/null)
# macOS bundle metadata uses x.y.z. Keep the full canonical string available to
# Rust as TARIBOY_VERSION while SEMVER owns the Cargo/Tauri comparison.
SEMVER      := $(firstword $(subst -, ,$(VERSION)))
DESKTOP_BINARIES := tariboyd tariboy tariboy-shim tariboy-plugin-telegram
# Resolved once so every desktop gate below agrees on what host it is running on.
HOST_OS     := $(shell uname -s)
HOST_ARCH   := $(shell uname -m)
NATIVE_PLATFORM := $(if $(filter Darwin-arm64,$(HOST_OS)-$(HOST_ARCH)),darwin,$(if $(filter Linux-x86_64,$(HOST_OS)-$(HOST_ARCH)),linux,unsupported))
PLATFORM    ?= $(NATIVE_PLATFORM)

# Desktop packaging is native: Tauri's platform bundlers are host tools rather
# than cross-compilers. Validate the public selector before checking or
# installing tools, so an invalid request has no external side effects.
desktop-platform-check:
	@case "$(PLATFORM)" in \
		darwin|linux) ;; \
		*) echo "PLATFORM must be darwin or linux, found $(PLATFORM) on $(HOST_OS)-$(HOST_ARCH)" >&2; exit 1 ;; \
	esac
	@if [ "$(NATIVE_PLATFORM)" = "unsupported" ]; then \
		echo "PLATFORM supports darwin on Darwin-arm64 or linux on Linux-x86_64, found $(HOST_OS)-$(HOST_ARCH)" >&2; \
		exit 1; \
	fi
	@if [ "$(PLATFORM)" != "$(NATIVE_PLATFORM)" ]; then \
		echo "PLATFORM=$(PLATFORM) does not match native $(NATIVE_PLATFORM) on $(HOST_OS)-$(HOST_ARCH); accepted values are darwin and linux" >&2; \
		exit 1; \
	fi
	@echo "host ok: $(HOST_OS)-$(HOST_ARCH) (PLATFORM=$(PLATFORM))"

# Tooling the Rust side needs beyond the Go toolchain. cargo and npm must be
# installed by hand — they are language runtimes, not project state. tauri-cli is
# project state (a pinned cargo subcommand that lands in the user's cargo bin),
# so install it rather than making the first desktop build fail on it.
desktop-tools-check: desktop-platform-check
	@command -v cargo >/dev/null || { \
		echo "cargo not found — install the Rust toolchain first:" >&2; \
		echo "  curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs | sh" >&2; \
		exit 1; \
	}
	@command -v npm >/dev/null || { \
		echo "npm not found — install Node.js first (the SPA build needs it)" >&2; \
		exit 1; \
	}
	@if ! cargo tauri --version >/dev/null 2>&1; then \
		echo "tauri-cli missing; installing the pinned version (this takes a few minutes)"; \
		cargo install tauri-cli --version "^2" --locked || exit 1; \
	fi
	@echo "tools ok: $$(cargo --version), $$(cargo tauri --version), npm $$(npm --version)"

desktop-preflight: desktop-tools-check

# The app's "is the running daemon the one I bundle?" banner is only meaningful
# while these three agree, so a drift is a build failure, not a warning.
desktop-version-check:
	@test -n "$(VERSION)" || { echo "cannot read Version from internal/version/version.go" >&2; exit 1; }
	@test -n "$(RELEASE_VERSION)" || { echo "cannot read declared release version from scripts/release-version.txt" >&2; exit 1; }
	@test "$(VERSION)" = "$(RELEASE_VERSION)" || { echo "release version must be $(RELEASE_VERSION), found $(VERSION)" >&2; exit 1; }
	@cargo_v=$$(sed -n 's/^version = "\(.*\)"$$/\1/p' $(TAURI_DIR)/Cargo.toml | head -1); \
	conf_v=$$(sed -n 's/.*"version": "\(.*\)".*/\1/p' $(TAURI_DIR)/tauri.conf.json | head -1); \
	if [ "$$cargo_v" != "$(SEMVER)" ] || [ "$$conf_v" != "$(SEMVER)" ]; then \
		echo "version drift: internal/version=$(VERSION) (semver $(SEMVER)) Cargo.toml=$$cargo_v tauri.conf.json=$$conf_v" >&2; \
		exit 1; \
		fi
	@lock_v=$$(python3 -c 'import tomllib; packages=tomllib.load(open("$(TAURI_DIR)/Cargo.lock", "rb")).get("package", []); print(next((p["version"] for p in packages if p.get("name") == "tariboy-desktop"), ""))'); \
	if [ "$$lock_v" != "$(SEMVER)" ]; then \
		echo "version drift: $(TAURI_DIR)/Cargo.lock package tariboy-desktop expected $(SEMVER), found $$lock_v" >&2; \
		exit 1; \
	fi
	@python3 -c 'import json; c=json.load(open("$(TAURI_DIR)/tauri.conf.json")); assert c["productName"] == "Tariboy"; assert c["app"]["windows"][0]["title"] == "Tariboy"'
	@rg -q 'Open Tariboy' $(TAURI_DIR)/src/menu.rs
	@rg -q 'Quit Tariboy' $(TAURI_DIR)/src/menu.rs
	@rg -q 'Tariboy cannot start' $(TAURI_DIR)/src/main.rs
	@echo "version ok: $(VERSION)"

# Validate the committed Cargo.lock before any Rust build. Offline first, so a
# warm checkout never reaches the network. A cold ~/.cargo has none of the
# pinned crates on disk and cargo cannot emit metadata for packages it does not
# have, so the offline probe fails for a reason that has nothing to do with the
# lock — fetch exactly what the lock pins, then re-validate offline. Note that
# Cargo.lock carries every target platform's dependencies (chrono pulls
# iana-time-zone, which lists the android/haiku/wasm/windows backends next to
# the macOS one), so a cold cache trips on a crate this host never compiles.
# `cargo fetch --locked` still refuses to rewrite the lock, so a genuinely stale
# lock fails the build here instead of being silently repaired.
desktop-lock-check:
	@cd $(TAURI_DIR) && \
	if cargo metadata --format-version 1 --locked --offline >/dev/null 2>&1; then \
		echo "cargo lock ok (registry cache warm)"; \
	else \
		echo "cargo registry cache is cold; fetching the crates Cargo.lock pins"; \
		cargo fetch --locked || { \
			echo "cargo fetch failed — needs network on the first build, or Cargo.lock is stale" >&2; \
			exit 1; \
		}; \
		cargo metadata --format-version 1 --locked --offline >/dev/null || exit 1; \
		echo "cargo lock ok (crates fetched)"; \
	fi

# Build both binary sets without packaging, useful for tests and remote smoke.
desktop-binaries: desktop-version-check build-basic-image
	rm -rf desktop/src-tauri/resources/bin/darwin-arm64 desktop/src-tauri/resources/bin/linux-x86_64
	mkdir -p $(DESKTOP_DARWIN_BIN) $(DESKTOP_LINUX_BIN)
	@for b in $(DESKTOP_BINARIES); do \
		echo "build darwin/arm64 $$b"; \
		CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 $(GO) build -trimpath -o $(DESKTOP_DARWIN_BIN)/$$b ./cmd/$$b || exit 1; \
	done
	@for b in $(DESKTOP_BINARIES); do \
		echo "build linux/amd64 $$b"; \
		CGO_ENABLED=0 GOOS=linux GOARCH=amd64 $(GO) build -trimpath -o $(DESKTOP_LINUX_BIN)/$$b ./cmd/$$b || exit 1; \
	done
	@printf '%s\n' "$(VERSION)" > $(DESKTOP_DARWIN_BIN)/VERSION
	@printf '%s\n' "$(VERSION)" > $(DESKTOP_LINUX_BIN)/VERSION
	cp $(TAURI_DIR)/src/remote-install.sh $(DESKTOP_LINUX_BIN)/remote-install.sh
	chmod 0755 $(DESKTOP_LINUX_BIN)/remote-install.sh
	@cd $(DESKTOP_LINUX_BIN) && shasum -a 256 $(DESKTOP_BINARIES) > SHA256SUMS
	@case "$$(uname -s)-$$(uname -m)" in \
		Darwin-arm64) native_dir=$(DESKTOP_DARWIN_BIN) ;; \
		Linux-x86_64) native_dir=$(DESKTOP_LINUX_BIN) ;; \
		*) echo "cannot execute a bundled target on this build host" >&2; exit 1 ;; \
	esac; \
	for b in $(DESKTOP_BINARIES); do \
		bundled=$$($$native_dir/$$b --version); \
		if [ "$$bundled" != "$(VERSION)" ]; then \
			echo "bundled $$native_dir/$$b reports $$bundled, expected $(VERSION)" >&2; exit 1; \
		fi; \
	done
	@for platform in darwin-arm64 linux-x86_64; do \
		case "$$platform" in \
			darwin-arm64) expected_format='Mach-O 64-bit.*arm64' ;; \
			linux-x86_64) expected_format='ELF 64-bit.*x86-64' ;; \
		esac; \
		for b in $(DESKTOP_BINARIES); do \
			path=$(DESKTOP_BIN)/$$platform/$$b; \
			file "$$path" | grep "$$expected_format" >/dev/null || { \
				echo "$$path has the wrong architecture" >&2; exit 1; \
			}; \
			strings "$$path" | grep -F "$(VERSION)" >/dev/null || { \
				echo "$$path does not embed $(VERSION)" >&2; exit 1; \
			}; \
			if strings "$$path" | grep -F "0.9.0-dev" >/dev/null; then \
				echo "$$path still embeds 0.9.0-dev" >&2; exit 1; \
			fi; \
		done; \
	done

# Build a native production Desktop package. Both platforms always rebuild the
# SPA and bundled binaries so a package cannot ship stale UI or daemon payloads.
# Darwin uses Tauri's headless-safe DMG path; Linux produces both supported
# package formats without depending on the separate WebDriver E2E toolchain.
desktop: desktop-preflight desktop-binaries desktop-lock-check
	@if [ ! -d ui/node_modules ]; then \
		if [ "$(DESKTOP_INSTALL_UI_DEPS)" = "1" ]; then \
			echo "ui/node_modules is missing; installing locked UI dependencies"; \
			cd ui && npm ci; \
		else \
			echo "ui/node_modules is missing, run: cd ui && npm ci" >&2; \
			exit 1; \
		fi; \
	fi
	$(MAKE) ui
ifeq ($(PLATFORM),darwin)
	cd $(TAURI_DIR) && CI=true TARIBOY_VERSION="$(VERSION)" cargo tauri build --bundles app,dmg
	@echo "app: $(TAURI_DIR)/target/release/bundle/macos/Tariboy.app"
	@echo "dmg: $(TAURI_DIR)/target/release/bundle/dmg/"
else ifeq ($(PLATFORM),linux)
	cd $(TAURI_DIR) && TARIBOY_VERSION="$(VERSION)" cargo tauri build --bundles deb,appimage
	@echo "deb: $(TAURI_DIR)/target/release/bundle/deb/"
	@echo "AppImage: $(TAURI_DIR)/target/release/bundle/appimage/"
else
	@echo "PLATFORM=$(PLATFORM) is invalid; accepted values are darwin and linux on $(HOST_OS)-$(HOST_ARCH)" >&2
	@exit 1
endif

# Host binaries only the Desktop E2E run needs. ui/tests/desktop/fixture.ts
# resolves them in worker scope — after the Go binaries, the SPA and the Tauri
# debug build are already built — so a missing driver used to cost a full build
# and then repeat the same line once per spec. Checking here, as the first
# prerequisite of desktop-e2e-build, turns that into one line before the first
# expensive step. This deliberately does not live in desktop-tools-check: macOS
# packaging goes make desktop -> desktop-preflight -> desktop-tools-check and
# has no use for a WebDriver, so requiring one there would break it.
# The lookup rule mirrors fixture.ts executable() exactly: when the override
# variable is set it is the only candidate, otherwise the bare name is searched
# in PATH. Xvfb is only required when DISPLAY is empty, matching startDisplay() —
# on a host with a live X server the fixture never looks for it.
desktop-e2e-tools-check:
	@check() { \
		name=$$1; var=$$2; override=$$3; hint=$$4; \
		if [ -n "$$override" ]; then \
			if [ -x "$$override" ]; then echo "  $$name: $$override (from $$var)"; return 0; fi; \
			echo "$$name is required for make desktop-e2e: $$var=$$override is not an executable file" >&2; \
		else \
			found=$$(command -v "$$name" 2>/dev/null); \
			if [ -n "$$found" ] && [ -x "$$found" ]; then echo "  $$name: $$found"; return 0; fi; \
			echo "$$name is required for make desktop-e2e and was not found in PATH" >&2; \
		fi; \
		echo "  install it with: $$hint" >&2; \
		echo "  or point $$var at an existing executable" >&2; \
		return 1; \
	}; \
	check tauri-driver TAURI_DRIVER_BIN "$$TAURI_DRIVER_BIN" "cargo install tauri-driver" || exit 1; \
	check WebKitWebDriver WEBKIT_WEBDRIVER_BIN "$$WEBKIT_WEBDRIVER_BIN" "sudo apt-get install -y webkit2gtk-driver" || exit 1; \
	if [ -n "$$DISPLAY" ]; then \
		echo "  Xvfb: not needed, DISPLAY=$$DISPLAY is already set"; \
	else \
		check Xvfb TARIBOY_XVFB_BIN "$$TARIBOY_XVFB_BIN" "sudo apt-get install -y xvfb" || exit 1; \
	fi; \
	echo "desktop e2e tools ok"

# Build and exercise the real Linux Tauri executable through tauri-driver.
# Packaging is deliberately skipped: WebDriver needs the executable, not a
# .deb or AppImage artifact.
desktop-e2e-build: desktop-e2e-tools-check desktop-tools-check desktop-binaries desktop-lock-check
	$(MAKE) ui
	cd $(TAURI_DIR) && TARIBOY_VERSION="$(VERSION)" cargo tauri build --debug --no-bundle

desktop-e2e: desktop-e2e-build
	cd ui && npm run test:desktop-e2e -- --workers=$(DESKTOP_E2E_WORKERS) $(DESKTOP_E2E_ARGS)

# Produce the reviewed, static internal-alpha release directory. Packaging is a
# macOS Apple Silicon operation; portable version/syntax gates remain available
# through desktop-version-check and bash -n on every development host.
desktop-alpha:
	./scripts/package-alpha.sh

# Isolated integration check for the packaged app. Never touches the live daemon:
# the script mktemp's both TARIBOY_BASE_DIR and TARIBOY_RUNTIME_DIR, and
# the app picks a free port because 9990 is normally already taken.
desktop-smoke:
	./scripts/desktop-smoke.sh

# ---- one-command entry points: `check` (fast) and `full-check` (heavy) ----
#
# The checks in this project are many and live in three places: Make targets, npm
# scripts under ui/ and docs/, and standalone scripts under scripts/. Nobody can
# hold that list in their head, so these two targets are the list. They COMPOSE
# the existing targets, they do not replace them: every brick above stays exactly
# as it is and stays callable on its own.
#
# `check` is the fast one and is safe to run in a shared working tree: it only
# reads. It never rewrites files (`fmt` is deliberately NOT part of it —
# fmt-check is), never installs node modules, and never writes into $(BINDIR).
# `full-check` is the heavy one: it builds, runs the e2e scripts, full-smoke, the
# browser suites and the desktop gates.
#
# Sub-makes are spelled $(SUBMAKE), not $(MAKE), on purpose. GNU make executes a
# recipe line that literally contains "$(MAKE)" even under -n, and both recipes
# below are ONE shell line — so spelling it $(MAKE) would make `make -n
# full-check` really run the e2e scripts instead of printing them. Going through
# a variable keeps -n a dry run. The cost is that a sub-make cannot join a `make
# -j` jobserver; these entry points are sequential by design, so nothing is lost.
SUBMAKE := $(MAKE) --no-print-directory

# The single mechanism behind both targets. run_step NAME CMD runs CMD in a
# subshell, times it, and records ok/FAIL — it never aborts the run, because one
# pass has to surface gofmt drift AND a ui type error at once, not one per pass.
# summarize prints the table and owns the exit status. There is no fail-fast flag
# by design. The subshell is what makes `cd ui && ...` steps safe to chain, and
# eval (rather than sh -c) is what keeps need_node_modules visible inside it.
define STEP_RUNNER
set +e; \
rows=""; failed=0; \
need_node_modules() { \
	if [ ! -d "$$1/node_modules" ]; then \
		echo "$$1/node_modules is missing, run: cd $$1 && npm ci" >&2; \
		return 1; \
	fi; \
}; \
run_step() { \
	printf '\n==> %s\n' "$$1"; \
	__start=$$(date +%s); \
	( eval "$$2" ); \
	__code=$$?; \
	__secs=$$(( $$(date +%s) - $$__start )); \
	if [ "$$__code" -eq 0 ]; then __st=ok; else __st=FAIL; failed=1; fi; \
	rows="$$rows$$(printf '%-28s %-4s %5ss' "$$1" "$$__st" "$$__secs")\n"; \
}; \
summarize() { \
	printf '\n==> %s summary\n' "$$1"; \
	printf '%b' "$$rows"; \
	if [ "$$failed" -ne 0 ]; then \
		printf '%s FAILED\n' "$$1" >&2; \
		exit 1; \
	fi; \
	printf '%s ok\n' "$$1"; \
}
endef

check:
	@$(STEP_RUNNER); \
	run_step "fmt-check"    '$(SUBMAKE) fmt-check'; \
	run_step "vet"          '$(SUBMAKE) vet'; \
	run_step "test"         '$(SUBMAKE) test'; \
	run_step "store-skills" 'PYTHONDONTWRITEBYTECODE=1 python3 store/skills/test_store_skills.py'; \
	run_step "smoke-contract" '$(SUBMAKE) smoke-contract-test'; \
	run_step "ui-typecheck" 'need_node_modules ui && cd ui && npx tsc -b'; \
	run_step "ui-lint"      'need_node_modules ui && cd ui && npm run lint'; \
	run_step "ui-test"      'need_node_modules ui && cd ui && npm test'; \
	run_step "ui-branding"  'need_node_modules ui && cd ui && npm run branding:check'; \
	run_step "docs"         'need_node_modules docs && cd docs && npm run doctor && npm run build && ../scripts/docs-build-contract-test.sh'; \
	summarize check

# The desktop tail of `full-check` is one step per host, not two, and that is
# deliberate. desktop-e2e depends on desktop-e2e-build, which depends on
# desktop-binaries (-> desktop-version-check) and on desktop-lock-check, so
# running the two gates as their own step and then calling desktop-e2e would run
# both gates twice. One make invocation with several goals builds each target at
# most once, which is exactly the guarantee we want here.
ifeq ($(HOST_OS)-$(HOST_ARCH),Linux-x86_64)
DESKTOP_STEP_NAME := desktop-e2e
DESKTOP_STEP_CMD  := $(SUBMAKE) desktop-version-check desktop-lock-check desktop-e2e
else ifeq ($(HOST_OS)-$(HOST_ARCH),Darwin-arm64)
DESKTOP_STEP_NAME := desktop
DESKTOP_STEP_CMD  := $(SUBMAKE) DESKTOP_INSTALL_UI_DEPS=0 desktop-version-check desktop-lock-check desktop desktop-smoke
else
DESKTOP_STEP_NAME := desktop-checks
DESKTOP_STEP_CMD  := $(SUBMAKE) desktop-version-check desktop-lock-check && echo "packaging step skipped, no desktop target for host $(HOST_OS)-$(HOST_ARCH)"
endif

# `build` is run once from here, explicitly, and every e2e script is then called
# DIRECTLY rather than through its make target: the targets are declared
# `e2e: build`, so repeated `$(SUBMAKE) *-e2e` calls would relink the five
# binaries each time. (The full-smoke step still rebuilds on its own — the script runs
# `make build` internally so it also works when launched by hand.)
#
# STEP ORDER IS DELIBERATE: the desktop step must stay LAST. On Linux x86_64 and
# macOS arm64 the step reaches `ui` — desktop (macOS arm64) and
# desktop-e2e-build (Linux x86_64) both call `$(MAKE) ui`. A standalone
# `make desktop` bootstraps missing UI dependencies, but the macOS full-check
# command disables that bootstrap so this gate keeps the same prepared
# `ui/node_modules` contract as `check`. This keeps the full gate read-only with
# respect to Node dependencies and confines a missing dependency failure to the
# final desktop row. Do not reorder these lines for speed: the desktop step
# remains last to keep generated desktop artifacts out of the earlier checks.
full-check:
	@$(STEP_RUNNER); \
	run_step "check"                     '$(SUBMAKE) check'; \
	run_step "build"                     '$(SUBMAKE) build'; \
	run_step "e2e"                       './scripts/e2e.sh'; \
	run_step "workflow-e2e"              './scripts/workflow-e2e.sh'; \
	run_step "iteration-timeout-e2e"     './scripts/iteration-timeout-e2e.sh'; \
	run_step "group-request-deadline-e2e" './scripts/group-request-deadline-e2e.sh'; \
	run_step "full-smoke"                '$(SUBMAKE) full-smoke'; \
	run_step "ui-tasks-browser"          'need_node_modules ui && cd ui && npm run test:tasks-browser'; \
	run_step "ui-workspace-browser"      'need_node_modules ui && cd ui && npm run test:workspace-browser'; \
	run_step "$(DESKTOP_STEP_NAME)"      '$(DESKTOP_STEP_CMD)'; \
	summarize full-check
