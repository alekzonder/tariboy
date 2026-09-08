# Streaming File Upload Design

## Goal

Allow Desktop file sends and `tariboy cp LOCAL_FILE` to upload files up to
1 GiB without materializing the complete file as JSON/base64 in either client
or daemon memory.

## Compatibility

- Keep `PUT /api/files` as the existing JSON/base64 operator command with its
  original 16 MiB decoded-file limit for older clients and direct
  `tariboy files upload` users.
- Add `PUT /api/files/raw?name=<encoded-file-name>` for a raw
  `application/octet-stream` request body up to 1 GiB.
- Preserve the successful response envelope and fields: `path`, `abs`, and
  `bytes`.

## Server flow

The API server owns the raw route because registry command dispatch decodes
JSON before invoking a command. It rejects a declared `Content-Length` above
1 GiB before reading the body, then streams at most 1 GiB plus one sentinel
byte into a newly created owner-only file beneath
`$TARIBOY_BASE_DIR/files/<unique-directory>/`.

One shared save function validates that `name` is a single filename without
path separators or control characters, confines writes with `os.Root`, creates
directories with mode `0700`, and creates the file with mode `0600`. Any read,
write, close, cancellation, or size-limit failure removes the partial file and
its unique directory. Unknown-length bodies are accepted but remain bounded by
the streaming limit.

The legacy JSON handler keeps its base64 validation and 16 MiB bound, then uses
the same save function so both routes retain identical path validation,
permissions, confinement, unique naming, and cleanup behavior.

## Clients

Desktop sends the browser `File` object directly to the raw endpoint. The
existing explicit-daemon target and bearer-token handling remain unchanged;
Send files, Attach, and terminal drop already share this helper.

`tariboy cp LOCAL_FILE` opens the local file, reads its size from metadata, and
passes the open reader to the Unix-socket HTTP client. Compose archive import
keeps its existing gzip upload method. No new dependency or upload protocol is
introduced.

## Errors

- Missing or unsafe names return `bad_path`.
- Bodies larger than 1 GiB return HTTP 413 with `too_large`.
- Legacy invalid base64 remains `bad_content`.
- Internal filesystem failures retain the existing internal-error envelope.

## Verification

TDD covers raw request metadata and body streaming from the UI and CLI,
declared oversize rejection before body reads, unknown-length overflow cleanup,
legacy JSON compatibility, filesystem confinement, and successful files above
the former 16 MiB ceiling. Focused Go and UI tests run during implementation;
the final branch gate is `make check` plus `git diff --check`. Per customer
instruction, `make full-check` is not run.
