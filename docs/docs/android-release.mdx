---
title: Android release APK
description: Build the signed Android APK locally and configure the GitHub Secrets and variable that let each release attach it.
sidebar:
  label: Android release APK
  icon: smartphone
---

Each GitHub Release attaches `Tariboy_X.Y.Z_android-arm64.apk`, signed with
the project's release key, and its `.sha256` file. This page tells you how the
APK is built and signed, how to build it on your computer, and which values you
must add to GitHub before the next release tag.

## How a release builds the APK

When you push a `vX.Y.Z` tag, `.github/workflows/desktop-release.yml` runs the
`android` job on `ubuntu-latest` in parallel with the macOS job. The job:

1. Rejects the tag when it differs from `internal/version/version.go` or
   `scripts/release-version.txt`.
2. Stops with `NAME is required` when one of the four `ANDROID_*` Secrets is
   empty.
3. Decodes the keystore into the runner's temporary directory and writes
   `desktop/src-tauri/gen/android/keystore.properties`. Only this step can
   read the Secrets.
4. Runs `make desktop-android`. Gradle signs the release APK with the key.
5. Checks the APK and stops the release when any check fails:
   - exactly one APK exists and its name does not end in `-unsigned.apk`;
   - `apksigner verify --print-certs` accepts the signature;
   - the certificate is not the Android debug certificate (`CN=Android Debug`);
   - the APK carries exactly one signing certificate, and its SHA-256
     fingerprint equals the `ANDROID_CERT_SHA256` repository variable.
     `scripts/apk-cert-sha256.sh` reads the fingerprint from both `apksigner`
     formats: `Signer #1 certificate SHA-256 digest:` and the per-scheme
     `V2 Signer: certificate SHA-256 digest:`.
6. Renames the APK to `Tariboy_X.Y.Z_android-arm64.apk` and writes its
   `.sha256` file.

The `publish` job waits for both the macOS and the Android job. It creates a
draft release, writes the Android note with the certificate fingerprint into
the release text, uploads all files, and then makes the release public. When
the Android job fails, no release is published, also not the macOS files.

## GitHub settings

You do these steps one time, before the next release tag. Without them the
`android` job fails and the release is not published.

| Name | Kind | Value |
| --- | --- | --- |
| `ANDROID_KEYSTORE_BASE64` | Secret | the keystore file, base64-encoded on one line |
| `ANDROID_KEYSTORE_PASSWORD` | Secret | the keystore password |
| `ANDROID_KEY_ALIAS` | Secret | the key alias, for example `tariboy` |
| `ANDROID_KEY_PASSWORD` | Secret | the key password; in a PKCS12 keystore it is the keystore password |
| `ANDROID_CERT_SHA256` | Variable | the SHA-256 fingerprint of the signing certificate |

The fingerprint is public information, so it is a variable and not a Secret.
The workflow ignores colons, spaces, and letter case in it.

### 1. Create the signing key

You need a JDK with `keytool`:

```bash
keytool -genkeypair -v -keystore tariboy-release.jks -storetype PKCS12 \
  -keyalg RSA -keysize 4096 -validity 10000 -alias tariboy
```

`keytool` asks for a password and for the owner's name. Keep
`tariboy-release.jks` and its password in a safe place, for example a password
manager. Do not commit the file to the repository.

Android does not update an installed APK with an APK signed by a different key.
If you lose or replace the key, users must uninstall the app and install it
again.

### 2. Read the fingerprint

```bash
keytool -list -v -keystore tariboy-release.jks -alias tariboy | grep 'SHA256:'
```

The output looks like `SHA256: 5F:A1:8B:...:30:A9`. Copy the value after
`SHA256:`.

### 3. Add the Secrets and the variable

In the browser:

1. Open the repository on GitHub and go to **Settings → Secrets and variables →
   Actions**.
2. On the **Secrets** tab, click **New repository secret** one time for each of
   the four `ANDROID_*` Secrets in the table. For `ANDROID_KEYSTORE_BASE64`,
   paste the output of `base64 < tariboy-release.jks | tr -d '\n'`.
3. On the **Variables** tab, click **New repository variable**, enter the name
   `ANDROID_CERT_SHA256`, and paste the fingerprint from step 2.

Or with the GitHub CLI, from the directory with the keystore. The commands
without `--body` ask for the value, so the password does not go into your shell
history:

```bash
base64 < tariboy-release.jks | tr -d '\n' | gh secret set ANDROID_KEYSTORE_BASE64
gh secret set ANDROID_KEYSTORE_PASSWORD
gh secret set ANDROID_KEY_ALIAS --body tariboy
gh secret set ANDROID_KEY_PASSWORD
gh variable set ANDROID_CERT_SHA256 --body '5F:A1:8B:...:30:A9'
```

### 4. Check the settings

```bash
gh secret list
gh variable list
```

`gh secret list` must show the four `ANDROID_*` names. `gh variable list` must
show `ANDROID_CERT_SHA256`. Nothing else is necessary: the runner has the
Android SDK, the NDK, and Java, and the workflow sets its own permissions.

When you replace the key, update all four Secrets and `ANDROID_CERT_SHA256`
together.

## Build the APK locally

You need a JDK 17 or newer, the Android SDK with a platform and build-tools in
`ANDROID_HOME`, and an NDK in `NDK_HOME`:

```bash
export ANDROID_HOME=$HOME/Android/Sdk
export NDK_HOME=$ANDROID_HOME/ndk/27.2.12479018
make desktop-android
```

The APK is in
`desktop/src-tauri/gen/android/app/build/outputs/apk/universal/release/`.
Without a keystore the file is `*-unsigned.apk`, which is good for local checks
only. To sign it, write `desktop/src-tauri/gen/android/keystore.properties`
(the file is ignored by Git):

```properties
storeFile=/absolute/path/to/tariboy-release.jks
storePassword=KEYSTORE_PASSWORD
keyAlias=tariboy
keyPassword=KEY_PASSWORD
```

## Verify a released APK

Download the APK and its `.sha256` file into one directory, then:

```bash
sha256sum -c Tariboy_X.Y.Z_android-arm64.apk.sha256
apksigner verify --print-certs Tariboy_X.Y.Z_android-arm64.apk
```

The `certificate SHA-256 digest` line must show the fingerprint in the release
text.

## When the Android job fails

| Message | Cause and fix |
| --- | --- |
| `ANDROID_... is required` | A Secret is missing. Add it as in step 3. |
| `ANDROID_CERT_SHA256 is required` | The variable is missing. Add it as in step 3. |
| `does not match ANDROID_CERT_SHA256` | The keystore and the variable are from different keys. Set the variable to the keystore's fingerprint. |
| `expected one APK signing certificate SHA-256, found N` | `apksigner` printed no certificate digest, or the APK is signed with more than one certificate. Read the `apksigner` output printed above the message. |
| `release APK is signed with a debug certificate` | The keystore holds the Android debug key. Use the release keystore. |
| `make desktop-android` fails in the Gradle signing task | The password or the alias does not match the keystore. Check the Secrets. |
| `release APK is unsigned` | Gradle did not read `keystore.properties`. Look at the `Restore signing keystore` step output. |

After the fix, open the failed run in **Actions** and click **Re-run failed
jobs**, or push a new release tag.

For how the phone app connects to a daemon, see
[Remote hosts](/remote-hosts#android-app).
