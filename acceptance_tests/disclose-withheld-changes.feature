# @git scenarios need `git` on the test host (the local operand is set up as a
# real work tree). The runner skips them when git is absent.
@git
Feature: Say what was held back

  In order to trust that nothing I wanted was skipped without my knowing, I want
  csync to tell me which changes it held back because git ignores them.

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
