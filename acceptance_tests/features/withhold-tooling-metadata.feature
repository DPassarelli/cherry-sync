# @git scenarios need `git` on the test host (the local operand is set up as a
# real work tree). The runner skips them when git is absent.
Feature: Never sync tooling metadata

  In order to keep each side's repository and csync settings intact, I want
  csync to leave .git and its own .csync.toml out of every sync, and to say
  that it did.

  @git
  Scenario: The local .git directory is never offered for sync
    # git never lists its own .git directory as ignored, and syncing it would
    # overwrite the other side's repository state (HEAD, index, refs, hooks).
    Given a local git repository containing these files:
      """
      src/main.go
      README.md
      """
    And   an empty remote directory
    When  I run "csync ./project user@host:/project"
    Then  the reported actions should be:
      | action | path        |
      | create | README.md   |
      | create | src/main.go |

  @git
  Scenario: The .git exclusion is disclosed even when nothing is gitignored
    # git never lists .git, so without this its exclusion would go unmentioned.
    Given a local git repository containing these files:
      """
      src/main.go
      README.md
      """
    And   an empty remote directory
    When  I run "csync ./project user@host:/project"
    Then  the .git directory should be reported as excluded
    And   no withheld changes should be reported

  @git
  Scenario: A submodule's nested .git metadata is never offered for sync
    # A checked-out submodule's .git is a file (a "gitdir:" pointer), not a
    # directory.
    Given a local git repository containing these files:
      """
      README.md
      vendor/lib/lib.go
      vendor/lib/.git
      """
    And   an empty remote directory
    When  I run "csync ./project user@host:/project"
    Then  the reported actions should be:
      | action | path              |
      | create | README.md         |
      | create | vendor/lib/lib.go |

  @git @remote
  Scenario: Pull direction — a remote repository's .git is never offered for sync
    # The exclusion follows whichever side is read, not whichever side is local
    # (#103).
    Given a local directory containing these files:
      """
      README.md
      """
    And   a remote git repository containing these files:
      """
      README.md
      src/main.go
      """
    When  I run "csync user@host:/project ./project"
    Then  the reported actions should be:
      | action | path        |
      | create | src/main.go |

  @git @remote
  Scenario: Pull direction — the remote repository's .git exclusion is disclosed
    # csync never withholds a file without saying so.
    Given a local directory containing these files:
      """
      README.md
      """
    And   a remote git repository containing these files:
      """
      README.md
      src/main.go
      """
    When  I run "csync user@host:/project ./project"
    Then  the .git directory should be reported as excluded
    And   no withheld changes should be reported

  @remote
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

  @remote
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

  @git
  Scenario: A gitignored .csync.toml is disclosed once, not counted twice
    # A project with a saved target often gitignores .csync.toml too. Counted
    # twice, it would suggest more files were held back than really were.
    Given a local git repository containing these files:
      """
      src/main.go
      README.md
      """
    And   the repository's ".gitignore" contains:
      """
      *.log
      .csync.toml
      """
    And   a ".csync.toml" in the project directory containing:
      """
      remote = "user@host:/project"
      """
    And   that the file "debug.log" has been added locally
    When  I run "csync ./project user@host:/project"
    Then  the log should record the excluded paths:
      | debug.log |
    And   the .csync.toml file should be reported as excluded
