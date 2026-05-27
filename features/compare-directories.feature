Feature: Compare directories

  In order to understand what a sync will do before committing to it,
  I want to see which files differ and what action would be taken.

  Background:
    Given a local directory containing these files:
      """
      src/main.go
      src/parser.go
      README.md
      LICENSE
      .gitignore
      """

  Scenario: None of the files are different
    Given that all of the files are identical between local and remote
    When  I run "csync"
    Then  the output should be:
      """
      Source:      ./project
      Destination: user@host:/project

        (no differences)

      0 changes to make.
      """

  Scenario: One of the files is different
    Given that the file "README.md" has been changed locally
    When  I run "csync"
    Then  the output should be:
      """
      Source:      ./project
      Destination: user@host:/project

        update README.md

      1 change to make.
      """

  Scenario: Two of the files are different
    Given that the file "README.md" has been changed locally
    And   that the file "src/adder.go" has been added locally
    When  I run "csync"
    Then  the output should be:
      """
      Source:      ./project
      Destination: user@host:/project

        update README.md
        create src/adder.go

      2 changes to make.
      """

  # ---------------------------------------------------------------------------
  # TODO: Additional scenarios for this feature, not yet drafted.
  # Each will become a real Scenario block as we drill into it.
  # ---------------------------------------------------------------------------
  #
  # - Pull direction: invocation with a remote source and local destination
  #   (e.g. `csync user@host:/srv/project ./project`) infers a pull. The
  #   Source/Destination labels swap accordingly; action verbs (create / update
  #   / delete) still describe what happens at the destination.
  #
  # - No differences: when both sides are identical, the user sees a clear
  #   "No changes." message and the process exits 0.
  #
  # - Missing rsync: if `rsync` is not on PATH, csync exits with a clear,
  #   actionable error (does not produce a partial diff).
  #
  # - SSH connection fails: csync surfaces rsync's error verbatim rather than
  #   swallowing it; non-zero exit code.
  #
  # - Source path does not exist: csync reports the missing path clearly and
  #   exits non-zero, without attempting any remote connection.
  #
  # - Both paths local: decide whether local-to-local sync is supported in v0.1
  #   or explicitly rejected with a message pointing to plain rsync.
  #
  # - Both paths remote: explicitly rejected (csync uses SSH transport from the
  #   local machine; no daemon-mode support).
  #
  # - Trailing-slash semantics: decide whether csync preserves rsync's classic
  #   `./foo` vs `./foo/` distinction (pass-through) or normalizes it. The
  #   pass-through option keeps muscle memory intact for rsync users but
  #   carries the same footgun.
