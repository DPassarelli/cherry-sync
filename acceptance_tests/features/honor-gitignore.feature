# @git scenarios need `git` on the test host (the local operand is set up as a
# real work tree). The runner skips them when git is absent.
@git
Feature: Leave ignored files out of a sync

  In order to keep build artifacts and other ignored files out of a sync,
  I want files the local git repository ignores left out of the comparison,
  so the diff shows only files I'd actually consider moving.

  # The local repository's ignore rules govern in both directions, for push and
  # pull alike.

  Scenario: An ignored file is left out of the comparison
    Given a local git repository containing these files:
      """
      src/main.go
      README.md
      """
    And   the repository's ".gitignore" contains:
      """
      *.log
      """
    And   that the file "README.md" has been changed locally
    And   that the file "debug.log" has been added locally
    When  I run "csync ./project user@host:/project"
    Then  the reported actions should be:
      | action | path      |
      | update | README.md |
    And   the reported change count should be 1

  Scenario: A non-repository local side excludes nothing
    # Exclusion depends on being in a git work tree, not on a .gitignore being
    # present.
    Given a local directory containing these files:
      """
      src/main.go
      README.md
      """
    And   the directory's ".gitignore" contains:
      """
      *.log
      """
    And   that the file "README.md" has been changed locally
    And   that the file "debug.log" has been added locally
    When  I run "csync ./project user@host:/project"
    Then  the reported actions should be:
      | action | path      |
      | update | README.md |
      | create | debug.log |
    And   the reported change count should be 2
    And   no withheld changes should be reported
    And   the .git directory should not be reported as excluded

  Scenario: A file ignored via .git/info/exclude is left out
    # .git/info/exclude is git's per-clone ignore list, which is never committed.
    Given a local git repository containing these files:
      """
      src/main.go
      README.md
      """
    And   the repository's ".git/info/exclude" contains:
      """
      *.log
      """
    And   that the file "README.md" has been changed locally
    And   that the file "debug.log" has been added locally
    When  I run "csync ./project user@host:/project"
    Then  the reported actions should be:
      | action | path      |
      | update | README.md |
    And   the reported change count should be 1

  Scenario: A top-level ignore does not float onto a same-named nested path
    Given a local git repository containing these files:
      """
      src/main.go
      README.md
      src/build/keep.go
      build/artifact.o
      """
    And   the repository's ".gitignore" contains:
      """
      /build/
      """
    And   that the file "build/artifact.o" has been changed locally
    And   that the file "src/build/keep.go" has been changed locally
    When  I run "csync ./project user@host:/project"
    Then  the reported actions should be:
      | action | path              |
      | update | src/build/keep.go |
    And   the reported change count should be 1

  @remote
  Scenario: Pull direction — the local repo's ignore set still governs
    Given a local git repository containing these files:
      """
      src/main.go
      README.md
      debug.log
      """
    And   the repository's ".gitignore" contains:
      """
      *.log
      """
    And   that the file "debug.log" has been changed locally
    And   that the file "notes.txt" has been added on the remote
    When  I run "csync user@host:/project ./project"
    Then  the reported actions should be:
      | action | path      |
      | create | notes.txt |
    And   the reported change count should be 1

  @remote
  Scenario: Pull direction — a remote-only file the local repo ignores is held back
    Given a local git repository containing these files:
      """
      src/main.go
      README.md
      """
    And   the repository's ".gitignore" contains:
      """
      *.log
      """
    And   that the file "secret.log" has been added on the remote
    And   that the file "notes.txt" has been added on the remote
    When  I run "csync user@host:/project ./project"
    Then  the reported actions should be:
      | action | path      |
      | create | notes.txt |
    And   the reported change count should be 1
    And   the withheld changes should be:
      | action | path       |
      | create | secret.log |

  @remote
  Scenario: Pull direction — a folder of ignored files still offers a remote file that isn't ignored
    # A rule like logs/*.log ignores files, not the folder, even when every local
    # file in the folder matches it (#118).
    Given a local git repository containing these files:
      """
      src/main.go
      logs/old.log
      """
    And   the repository's ".gitignore" contains:
      """
      logs/*.log
      """
    And   that the file "logs/notes.txt" has been added on the remote
    When  I run "csync user@host:/project ./project"
    Then  the reported actions should be:
      | action | path           |
      | create | logs/notes.txt |

  @remote
  Scenario: Pull direction — a re-included file in a folder of ignored files is still offered
    # A re-include like this is a common way to keep a generated folder
    # documented.
    Given a local git repository containing these files:
      """
      src/main.go
      logs/old.log
      """
    And   the repository's ".gitignore" contains:
      """
      logs/*
      !logs/README.md
      """
    And   that the file "logs/README.md" has been added on the remote
    When  I run "csync user@host:/project ./project"
    Then  the reported actions should be:
      | action | path           |
      | create | logs/README.md |

  # ---------------------------------------------------------------------------
  # TODO: sibling scenarios, each its own behavior — drafted as we drill in.
  # ---------------------------------------------------------------------------
  #
  # - Syncing a *subdirectory* of the repo (csync ./repo/sub host:/dst): the
  #   emitted ignore paths must resolve relative to the sync root, not the repo
  #   root — `git ls-files` is run in the sync dir for exactly this reason. The
  #   anchoring half is covered above; this is the run-git-in-the-sync-dir half,
  #   still untested. Needs harness support for a sync operand below the repo
  #   root.
