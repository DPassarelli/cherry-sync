# @remote runs every scenario over a fake SSH remote. That is required, not
# incidental: measuring the destination takes a second rsync pass only when the
# destination is remote, and a local-to-local run would stat it instead and never
# exercise that pass at all.
@remote
Feature: Explain how each file differs

  In order to judge which copy of a file is worth keeping, I want the change
  list to tell me how the incoming copy compares with the one it would replace,
  rather than only that the two are not the same.

  Background:
    Given a local directory containing these files:
      """
      README.md
      src/main.go
      """
    And   that all of the files are identical between local and remote

  Scenario: An updated file reports its size gap and how old each copy is
    Given that the local copy of "README.md" is 2 KB larger than the remote copy
    And   that the local copy of "README.md" was last modified 3 hours ago
    And   that the remote copy of "README.md" was last modified 17 days ago
    When  I run "csync ./project user@host:/project"
    Then  the reported detail for "README.md" should be "source is 2.0 KB larger, last updated 3h ago · dest last updated 17d ago"

  Scenario: Copies of the same size omit the size comparison
    # With nothing to say about size, "source" attaches to the timestamp instead,
    # so the line still names whose age it is reporting.
    Given that the two copies of "README.md" differ in content but not in size
    And   that the local copy of "README.md" was last modified 4 minutes ago
    And   that the remote copy of "README.md" was last modified 2 days ago
    When  I run "csync ./project user@host:/project"
    Then  the reported detail for "README.md" should be "source last updated 4m ago · dest last updated 2d ago"

  Scenario: A file matching in size and timestamp differs in content alone
    # Nothing outward separates the two copies, so the destination pass (a
    # size+mtime quick check) reports nothing about this file and there is no age
    # to compare against. This is the state most easily mistaken for a bug, so it
    # is named outright rather than left blank.
    Given that the two copies of "README.md" differ in content but not in size
    And   that both copies of "README.md" carry the same modification time
    When  I run "csync ./project user@host:/project"
    Then  the reported detail for "README.md" should be "contents only"

  Scenario: A new file carries no comparison
    Given that the file "src/adder.go" has been added locally
    When  I run "csync ./project user@host:/project"
    Then  no detail should be reported for "src/adder.go"

  Scenario: A file being removed carries no comparison
    Given that the file "README.md" has been deleted locally
    When  I run "csync ./project user@host:/project"
    Then  no detail should be reported for "README.md"

  Scenario: A destination that cannot be measured still reports its changes
    # The measurement only annotates a row. A comparison that has already
    # succeeded must not be failed by an annotation that could not be gathered,
    # so the run carries on and the row falls back to naming which attributes
    # differ.
    Given that the file "README.md" has been changed locally
    And   a remote that answers the comparison but fails the measurement
    When  I run "csync ./project user@host:/project"
    Then  the reported change count should be 1
    And   csync should return exit code 0
    And   the reported detail for "README.md" should be "size and mtime"
