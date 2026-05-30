Feature: Invoke command

  In order to use csync against any local/remote pair without pre-configuration,
  I want to specify the source and destination as command-line arguments.

  Scenario: Push direction — local source, remote destination
    When I run "csync ./project user@host:/project"
    Then the reported source should be "./project"
    And  the reported destination should be "user@host:/project"

  Scenario: Pull direction — remote source, local destination
    When I run "csync user@host:/project ./project"
    Then the reported source should be "user@host:/project"
    And  the reported destination should be "./project"

  Scenario: No arguments — show usage and exit non-zero
    When I run "csync"
    Then csync should return exit code 2
    And  the reported usage should begin with "usage: csync"

  Scenario: One argument — show usage and exit non-zero
    When I run "csync ./project"
    Then csync should return exit code 2
    And  the reported usage should begin with "usage: csync"

  Scenario: Empty path argument — show usage and exit non-zero
    When I run "csync <empty> user@host:/project"
    Then csync should return exit code 2
    And  the reported usage should begin with "usage: csync"

  # ---------------------------------------------------------------------------
  # TODO: Additional scenarios for this feature, not yet drafted.
  # Each will become a real Scenario block as we drill into it.
  # ---------------------------------------------------------------------------
  #
  # - Usage-message content: once we settle what the rest of the message
  #   should include (synopsis line, brief description, pointer to `--help`),
  #   add scenarios asserting those fields beyond the `usage: csync` prefix.
  #
  # - Three or more arguments: rejected in v0.1 (we are not supporting multiple
  #   sources yet).
  #
  # - Both paths local (no `user@host:` prefix on either): decide if v0.1
  #   supports this or rejects it with a message pointing to plain rsync.
  #
  # - Both paths remote: rejected — csync uses local SSH transport, no
  #   daemon-mode support.
  #
  # - Arguments override configuration: when csync is configured with one pair
  #   but invoked with explicit arguments, the arguments win.
  #
  # - Trailing-slash semantics: decide whether csync preserves rsync's classic
  #   `./foo` vs `./foo/` distinction (pass-through) or normalizes it.
  #
  # - rsync argument injection (leading-dash paths): DONE. compare.Run now
  #   emits a `--` end-of-options separator before the paths, so a source/dest
  #   beginning with `-` (e.g. `-e`, `--rsh=…`) is a path, not an rsync option.
  #   Covered by compare.TestRsyncArgs_SeparatesOptionsFromPaths. (NB: never
  #   was shell injection — exec.Command takes no `sh -c`, so shell
  #   metacharacters are already inert.) Remaining, NOT closed by `--`:
  #
  #   - Empty-string path: DONE. cli.Parse now rejects an empty source or
  #     destination (would otherwise become "/" → rsync at the filesystem
  #     root). Covered by cli.TestParse_EmptyPath_ReturnsError and the scenario
  #     "Empty path argument — show usage and exit non-zero" above.
  #
  #   - Newline in a path/filename: harmless to compare.Run, but the v0.1
  #     transfer phase drives rsync via `--files-from`, which is newline-
  #     delimited. A filename containing a newline (legal on Unix) could
  #     smuggle extra entries into the files-from list. Address when that
  #     phase lands — consider `--from0` with NUL-delimited paths.
  #
  #   - Path literally `--`: with our added `--`, the user's `--` becomes a
  #     literal path named `--`. Edge case, likely harmless; note if revisited.
