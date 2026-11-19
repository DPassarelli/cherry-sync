# Release bare executables, not archives

Status: Accepted

## Decision

Publish each platform's `csync` as a bare executable. Do not wrap release assets in a `tar.gz` or `zip`.

## Why

csync builds as a single static file, and it carries its own license text (`csync --license`), so an archive holds nothing but the binary. Unpacking is just extra work, and the published checksums can describe the executable itself. Decided in [#84](https://github.com/DPassarelli/cherry-sync/issues/84).

The accepted costs:

- A release asset loses its executable bit, so installing takes a `chmod +x`, which the install instructions spell out.
- A notarization ticket cannot be stapled to a bare Mach-O. The first run of a downloaded macOS binary needs a network connection so Gatekeeper can check notarization online, and the release gate cannot confirm the ticket on a CI runner.

## Alternatives rejected

- **A `tar.gz` per platform** (the earlier format). It preserves the executable bit, but only by adding an unpack step for a single file.
- **Ship the license beside the binary in an archive.** Embedding it keeps the binary self-contained wherever it is copied.

## Revisit if

- A release must ship more than one file per platform, such as a man page or shell completions.
- A distribution channel the project adopts requires archived assets.
- A downloaded macOS binary must run on first launch without a network connection.
