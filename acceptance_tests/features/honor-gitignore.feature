# @git scenarios need `git` on the test host (the local operand is set up as a
# real work tree). The runner skips them when git is absent.
@git
Feature: Honor .gitignore when comparing

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

  Scenario: The comparison names the ignored change it withheld
    # There is no opt-out flag, so this disclosure is the user's only sign that a
    # file was held back.
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
    Then  the withheld changes should be:
      | action | path      |
      | create | debug.log |

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

  Scenario: An ignored file that changed is reported as withheld
    # An ignored file that would have moved is the exclusion most likely to
    # surprise a user (#59).
    Given a local git repository containing these files:
      """
      src/main.go
      .env
      """
    And   the repository's ".gitignore" contains:
      """
      .env
      """
    And   that the file ".env" has been changed locally
    When  I run "csync ./project user@host:/project"
    Then  the withheld changes should be:
      | action | path |
      | update | .env |

  Scenario: An ignored file that matches on both sides is not reported as withheld
    Given a local git repository containing these files:
      """
      src/main.go
      .env
      """
    And   the repository's ".gitignore" contains:
      """
      .env
      """
    And   that the file "src/main.go" has been changed locally
    When  I run "csync ./project user@host:/project"
    Then  no withheld changes should be reported

  Scenario: A change inside an ignored directory is not reported as withheld
    # Ignored directories are skipped outright, which made comparing a large one
    # about 5x faster. Nobody is surprised that build/ did not sync.
    Given a local git repository containing these files:
      """
      src/main.go
      build/output.bin
      """
    And   the repository's ".gitignore" contains:
      """
      build/
      """
    And   that the file "build/output.bin" has been changed locally
    When  I run "csync ./project user@host:/project"
    Then  no withheld changes should be reported

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
    And   the reported change count should be 2

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
    And   the reported change count should be 2

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

  @remote
  Scenario: Many withheld changes under one top-level folder are summarized
    # A remote-only .ansible/ folder once produced thousands of withheld rows that
    # buried the changes on offer. The summary is display-only: it does not claim
    # the folder itself is ignored.
    Given a local git repository containing these files:
      """
      src/main.go
      """
    And   the repository's ".gitignore" contains:
      """
      build/
      """
    And   that 11 files have been added on the remote under "build/a"
    When  I run "csync user@host:/project ./project"
    Then  the withheld changes should be summarized as:
      | folder | count | actions |
      | build/ | 11    | create  |

  @remote
  Scenario: Ten or fewer withheld changes under one folder are listed individually
    # Naming each file is what tells a user which one they were missing (#59).
    Given a local git repository containing these files:
      """
      src/main.go
      """
    And   the repository's ".gitignore" contains:
      """
      build/
      """
    And   that 10 files have been added on the remote under "build/a"
    When  I run "csync user@host:/project ./project"
    Then  the withheld changes should list 10 individual files

  @remote
  Scenario: A summary counts each action separately
    # logs/README.md is not ignored, so the folder itself is still compared.
    Given a local git repository containing these files:
      """
      src/main.go
      logs/README.md
      """
    And   the repository's ".gitignore" contains:
      """
      logs/*.log
      """
    And   that 6 files have been added on the remote under "logs"
    And   that 5 files have been added locally under "logs"
    When  I run "csync user@host:/project ./project"
    Then  the withheld changes should be summarized as:
      | folder | count | actions             |
      | logs/  | 11    | 6 create · 5 delete |

  @remote
  Scenario: The run log records every withheld file, even when summarized
    # The run log is where a user checks whether one particular file was held
    # back.
    Given a local git repository containing these files:
      """
      src/main.go
      """
    And   the repository's ".gitignore" contains:
      """
      build/
      """
    And   that 11 files have been added on the remote under "build/a"
    When  I run "csync user@host:/project ./project"
    Then  the log should record 11 excluded paths

  @remote
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
    And   the reported change count should be 1

  @remote
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
