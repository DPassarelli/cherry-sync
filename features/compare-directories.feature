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
    When  I run "csync ./project user@host:/project"
    Then  no actions should be reported
    And   the reported change count should be 0

  Scenario: One of the files is different
    Given that all of the files are identical between local and remote
    And   that the file "README.md" has been changed locally
    When  I run "csync ./project user@host:/project"
    Then  the reported actions should be:
      | action | path      |
      | update | README.md |
    And   the reported change count should be 1

  Scenario: Two of the files are different
    Given that all of the files are identical between local and remote
    And   that the file "README.md" has been changed locally
    And   that the file "src/adder.go" has been added locally
    When  I run "csync ./project user@host:/project"
    Then  the reported actions should be:
      | action | path         |
      | update | README.md    |
      | create | src/adder.go |
    And   the reported change count should be 2

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
