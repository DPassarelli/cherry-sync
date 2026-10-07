# @remote runs every scenario over a fake SSH remote, so push and pull transfer
# the way they would against a real host (see select-and-sync.feature for why
# that matters).
@remote
Feature: Saved sync targets

  In order to avoid retyping a remote I sync with often, I want to save it once
  in the project's .csync.toml and refer to it with `csync push` / `csync pull`.

  Scenario: Push resolves the saved remote as the destination
    Given a local directory containing these files:
      """
      src/main.go
      README.md
      """
    And   a ".csync.toml" in the project directory containing:
      """
      remote = "user@host:/project"
      """
    And   that the file "README.md" has been changed locally
    When  I run "csync push" from the project directory and respond with "a"
    Then  the reported sync count should be 1
    And   the file "README.md" should be identical between local and remote

  Scenario: Pull resolves the saved remote as the source
    Given a local directory containing these files:
      """
      src/main.go
      README.md
      """
    And   a ".csync.toml" in the project directory containing:
      """
      remote = "user@host:/project"
      """
    And   that the file "notes.txt" has been added on the remote
    When  I run "csync pull" from the project directory and respond with "a"
    Then  the reported sync count should be 1
    And   the file "notes.txt" should be identical between local and remote

  Scenario: Push reports the resolved source and destination
    Given a local directory containing these files:
      """
      README.md
      """
    And   a ".csync.toml" in the project directory containing:
      """
      remote = "user@host:/project"
      """
    When  I run "csync push" from the project directory
    Then  the reported source should be "."
    And   the reported destination should be "user@host:/project"

  Scenario: csync does not offer its own .csync.toml as a change
    # Like .git, csync's config is tooling metadata, never a file to sync. This
    # holds on any run, not just push and pull.
    Given a local directory containing these files:
      """
      README.md
      """
    And   a ".csync.toml" in the project directory containing:
      """
      remote = "user@host:/project"
      """
    And   that the file "README.md" has been changed locally
    When  I run "csync ./project user@host:/project" and respond with "n"
    Then  the reported actions should be:
      | action | path      |
      | update | README.md |
    And   the reported change count should be 1

  Scenario: csync discloses that it held back its .csync.toml
    # There is no opt-out, so csync must say what it held back.
    Given a local directory containing these files:
      """
      README.md
      """
    And   a ".csync.toml" in the project directory containing:
      """
      remote = "user@host:/project"
      """
    When  I run "csync ./project user@host:/project"
    Then  the .csync.toml file should be reported as excluded

  Scenario: A missing .csync.toml fails loudly and transfers nothing
    # There is no fallback to a default. This covers non-interactive use; at a
    # terminal, push and pull will instead offer to create the file (see
    # interactive-mode.feature).
    Given a local directory containing these files:
      """
      README.md
      """
    And   that the file "README.md" has been changed locally
    When  I run "csync push" from the project directory
    Then  csync should return a non-zero exit code
    And   the reported error should mention ".csync.toml"
    And   the file "README.md" should still differ between local and remote

  Scenario: A .csync.toml with no remote key is rejected
    # An empty config is not an implicit default.
    Given a local directory containing these files:
      """
      README.md
      """
    And   a ".csync.toml" in the project directory containing:
      """
      # no remote defined yet
      """
    And   that the file "README.md" has been changed locally
    When  I run "csync push" from the project directory
    Then  csync should return a non-zero exit code
    And   the reported error should mention ".csync.toml"
    And   the file "README.md" should still differ between local and remote

  Scenario: A .csync.toml with an empty remote value is rejected
    # A path from config gets the same validation as one from the command line.
    # See SECURITY.md.
    Given a local directory containing these files:
      """
      README.md
      """
    And   a ".csync.toml" in the project directory containing:
      """
      remote = ""
      """
    And   that the file "README.md" has been changed locally
    When  I run "csync push" from the project directory
    Then  csync should return a non-zero exit code
    And   the reported error should mention ".csync.toml"
    And   the file "README.md" should still differ between local and remote

  Scenario: Malformed TOML is rejected with a clear error
    Given a local directory containing these files:
      """
      README.md
      """
    And   a ".csync.toml" in the project directory containing:
      """
      remote = "user@host:/project
      """
    And   that the file "README.md" has been changed locally
    When  I run "csync push" from the project directory
    Then  csync should return a non-zero exit code
    And   the reported error should mention "invalid .csync.toml"
    And   the file "README.md" should still differ between local and remote

  Scenario: push rejects extra arguments
    # Otherwise `csync push ./project` would read as syncing a directory named
    # "push".
    When  I run "csync push ./project"
    Then  csync should return exit code 2
    And   the reported error should mention "'push' takes no arguments"

  Scenario: pull rejects extra arguments
    When  I run "csync pull ./project"
    Then  csync should return exit code 2
    And   the reported error should mention "'pull' takes no arguments"

  Scenario: A configured remote that looks like an rsync option is treated as a path
    # A path read from config must not get around the guard on option-looking
    # paths. See SECURITY.md.
    Given a local directory containing these files:
      """
      README.md
      """
    And   a ".csync.toml" in the project directory containing:
      """
      remote = "-e"
      """
    When  I run "csync pull" from the project directory
    Then  csync should return a non-zero exit code

  Scenario: A saved remote with a ~ home shortcut is normalized before use
    # rsync takes a remote "~" literally (#50). csync rewrites it as a path
    # relative to the login home, which rsync resolves the same way, and says so.
    Given a local directory containing these files:
      """
      README.md
      """
    And   a ".csync.toml" in the project directory containing:
      """
      remote = "user@host:~/project"
      """
    When  I run "csync pull" from the project directory
    Then  the reported source should be "user@host:project"
    And   csync should report that it rewrote "~/project"

  Scenario: A saved remote with a ~user home shortcut is rejected
    # No relative path reaches another user's home, so it is rejected up front
    # rather than failing partway through.
    Given a local directory containing these files:
      """
      README.md
      """
    And   a ".csync.toml" in the project directory containing:
      """
      remote = "user@host:~deploy/project"
      """
    When  I run "csync pull" from the project directory
    Then  csync should return a non-zero exit code
    And   the reported error should mention "~"

  Scenario: A trailing slash on the saved remote is normalized away in the display
    # csync adds its own trailing slash to mean "the folder's contents", so one
    # already in the config would double up.
    Given a local directory containing these files:
      """
      README.md
      """
    And   a ".csync.toml" in the project directory containing:
      """
      remote = "user@host:/project/"
      """
    When  I run "csync pull" from the project directory
    Then  the reported source should be "user@host:/project"

  # ---------------------------------------------------------------------------
  # TODO: Additional scenarios for this feature, not yet drafted.
  # Each will become a real Scenario block as we drill into it.
  # ---------------------------------------------------------------------------
  #
  # - Source display for push: v1 shows the local operand as ".". Decide whether
  #   to show the resolved absolute cwd instead (clearer in logs, longer line).
  #
  # - Discovery is cwd-only in v1: running `csync push` from a SUBDIRECTORY of a
  #   project whose .csync.toml sits at the root must NOT find it (it errors).
  #   When walk-up discovery lands, this flips to "found from a subdirectory".
