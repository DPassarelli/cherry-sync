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

  Scenario: Pull direction — a file is new on the remote
    Given that all of the files are identical between local and remote
    And   that the file "src/remote_only.go" has been added on the remote
    When  I run "csync user@host:/project ./project"
    Then  the reported actions should be:
      | action | path               |
      | create | src/remote_only.go |
    And   the reported change count should be 1

  Scenario: A file that differs only in modification time is not a change
    # A file whose content is byte-identical but whose mtime differs (e.g. git
    # checkout stamps a different mtime per machine) must not surface as a change.
    # rsync's size+mtime quick-check flags it for transfer; --checksum on the
    # compare pass settles it by content, so the row itemizes as `.f..t......`
    # (no content bit) and is dropped. Remove --checksum and this goes red:
    # README.md reports as a phantom "update".
    Given that all of the files are identical between local and remote
    And   that the file "README.md" has a different modification time but identical content
    When  I run "csync ./project user@host:/project"
    Then  no actions should be reported
    And   the reported change count should be 0

  Scenario: A source that looks like an rsync option is treated as a path
    # Security regression guard. rsyncArgs puts `--` before the operands, so an
    # option-looking source (here `-e`, rsync's remote-shell flag) reaches rsync
    # as a path. That path doesn't exist, so rsync errors and csync exits
    # non-zero. Delete the `--` and rsync would honor `-e` and exit 0 — flipping
    # this red. This asserts the guard's *behavior*, not its implementation.
    # See SECURITY.md.
    When  I run "csync -e ./project"
    Then  csync should return a non-zero exit code

  Scenario: A comparison that fails reports what rsync said
    # An exit code names neither the failing component nor the failure: 255 is ssh's
    # code passed through rsync, worn identically by a refused key, a changed host
    # key, and an unreachable host. What a user can act on is the sentence rsync
    # wrote on stderr, so csync repeats it rather than reducing the failure to a
    # number. A missing source is the failure this suite can raise without a broken
    # remote, and the diagnostic it draws names why the path could not be read.
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
