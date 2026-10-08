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
    When  I run "csync ./project user@host:/project"
    Then  no actions should be reported

  Scenario: One of the files is different
    Given that the file "README.md" has been changed locally
    When  I run "csync ./project user@host:/project"
    Then  the reported actions should be:
      | action | path      |
      | update | README.md |

  Scenario: Two of the files are different
    Given that the file "README.md" has been changed locally
    And   that the file "src/adder.go" has been added locally
    When  I run "csync ./project user@host:/project"
    Then  the reported actions should be:
      | action | path         |
      | update | README.md    |
      | create | src/adder.go |

  Scenario: Pull direction — a file is new on the remote
    Given that the file "src/remote_only.go" has been added on the remote
    When  I run "csync user@host:/project ./project"
    Then  the reported actions should be:
      | action | path               |
      | create | src/remote_only.go |

  Scenario: A file that differs only in modification time is not a change
    Given that the file "README.md" has a different modification time but identical content
    When  I run "csync ./project user@host:/project"
    Then  no actions should be reported

  Scenario: A comparison that fails reports what rsync said
    # An exit code can't tell a refused key from a changed host key or an
    # unreachable host (ssh exits 255 for all three). rsync's own message is what
    # a user can act on.
    Given a local source path that does not exist
    And   an empty remote directory
    When  I run "csync ./project user@host:/project"
    Then  csync should return a non-zero exit code
    And   csync should report the diagnostic rsync wrote

  # ---------------------------------------------------------------------------
  # TODO: Additional scenarios for this feature, not yet drafted.
  # Each will become a real Scenario block as we drill into it.
  # ---------------------------------------------------------------------------
  #
  # - Missing rsync: if `rsync` is not on PATH, csync exits with a clear,
  #   actionable error (does not produce a partial diff).
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
