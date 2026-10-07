# Scenarios are brought into the run one at a time as we implement them. Each
# not-yet-implemented scenario carries its own @wip tag (excluded via the
# "~@wip" tag filter the runner applies); drop a scenario's tag when we drill in.
#
# @remote runs every scenario here over a fake SSH remote, so transfers run the
# way they would against a real host. A local-to-local run reports every change
# in the same direction, which once hid a push-direction bug.
@remote
Feature: Select and sync files

  In order to move only the files I actually want, I want to review the
  differences, pick which ones to transfer, and have csync sync exactly those.

  Background:
    Given a local directory containing these files:
      """
      src/main.go
      src/parser.go
      README.md
      LICENSE
      .gitignore
      """

  Scenario: No differences — nothing to sync, no prompt
    When  I run "csync ./project user@host:/project"
    Then  the reported message should begin with "No changes"
    And   csync should return exit code 0

  Scenario: Accepting the default selects every change
    Given that the file "README.md" has been changed locally
    And   that the file "src/adder.go" has been added locally
    When  I run "csync ./project user@host:/project" and respond with "<empty>"
    Then  the reported sync count should be 2
    And   the file "README.md" should be identical between local and remote
    And   the file "src/adder.go" should be identical between local and remote

  Scenario: Selecting "a" selects every change
    Given that the file "README.md" has been changed locally
    And   that the file "src/adder.go" has been added locally
    When  I run "csync ./project user@host:/project" and respond with "a"
    Then  the reported sync count should be 2
    And   the file "README.md" should be identical between local and remote
    And   the file "src/adder.go" should be identical between local and remote

  Scenario: A filename containing non-ASCII bytes transfers intact
    # This first surfaced with a macOS screenshot, whose name carries a narrow
    # no-break space (U+202F).
    Given that the file "café.txt" has been added locally
    When  I run "csync ./project user@host:/project" and respond with "a"
    Then  the reported sync count should be 1
    And   the file "café.txt" should be identical between local and remote

  Scenario: A completed sync leaves nothing to re-sync
    Given that the file "README.md" has been changed locally
    And   that the file "src/adder.go" has been added locally
    When  I run "csync ./project user@host:/project" and respond with "a"
    And   I run "csync ./project user@host:/project" a second time
    Then  no actions should be reported
    And   the reported change count should be 0

  Scenario: Choosing a subset by number syncs only those files
    Given that the file "README.md" has been changed locally
    And   that the file "src/adder.go" has been added locally
    When  I run "csync ./project user@host:/project" and respond with "1"
    Then  the reported sync count should be 1
    And   the file "README.md" should be identical between local and remote
    And   the file "src/adder.go" should not exist on the remote

  Scenario: A different number selects a different change
    Given that the file "README.md" has been changed locally
    And   that the file "src/adder.go" has been added locally
    When  I run "csync ./project user@host:/project" and respond with "2"
    Then  the reported sync count should be 1
    And   the file "src/adder.go" should be identical between local and remote
    And   the file "README.md" should still differ between local and remote

  Scenario: Choosing none transfers nothing and exits cleanly
    Given that the file "README.md" has been changed locally
    When  I run "csync ./project user@host:/project" and respond with "n"
    Then  the reported sync count should be 0
    And   the file "README.md" should still differ between local and remote
    And   csync should return exit code 0

  Scenario: An unrecognized response is rejected without transferring
    Given that the file "README.md" has been changed locally
    When  I run "csync ./project user@host:/project" and respond with "wat"
    Then  csync should return a non-zero exit code
    And   the file "README.md" should still differ between local and remote

  Scenario: An out-of-range number is rejected like an unrecognized response
    Given that the file "README.md" has been changed locally
    When  I run "csync ./project user@host:/project" and respond with "2"
    Then  csync should return a non-zero exit code
    And   the file "README.md" should still differ between local and remote

  Scenario: A hyphen range selects an inclusive span of changes
    # Rows number in tree order: 1 LICENSE, 2 README.md, 3 src/main.go, 4 src/parser.go.
    Given that the file "LICENSE" has been changed locally
    And   that the file "README.md" has been changed locally
    And   that the file "src/main.go" has been changed locally
    And   that the file "src/parser.go" has been changed locally
    When  I run "csync ./project user@host:/project" and respond with "1-3"
    Then  the reported sync count should be 3
    And   the file "LICENSE" should be identical between local and remote
    And   the file "README.md" should be identical between local and remote
    And   the file "src/main.go" should be identical between local and remote
    And   the file "src/parser.go" should still differ between local and remote

  Scenario: A comma list selects exactly the named changes
    # Rows number in tree order: 1 LICENSE, 2 README.md, 3 src/main.go.
    Given that the file "LICENSE" has been changed locally
    And   that the file "README.md" has been changed locally
    And   that the file "src/main.go" has been changed locally
    When  I run "csync ./project user@host:/project" and respond with "1,3"
    Then  the reported sync count should be 2
    And   the file "LICENSE" should be identical between local and remote
    And   the file "src/main.go" should be identical between local and remote
    And   the file "README.md" should still differ between local and remote

  Scenario: A combined range and list selects the span plus the named change
    # Rows number in tree order: 1 LICENSE, 2 README.md, 3 src/main.go, 4 src/parser.go.
    Given that the file "LICENSE" has been changed locally
    And   that the file "README.md" has been changed locally
    And   that the file "src/main.go" has been changed locally
    And   that the file "src/parser.go" has been changed locally
    When  I run "csync ./project user@host:/project" and respond with "1-2,4"
    Then  the reported sync count should be 3
    And   the file "LICENSE" should be identical between local and remote
    And   the file "README.md" should be identical between local and remote
    And   the file "src/parser.go" should be identical between local and remote
    And   the file "src/main.go" should still differ between local and remote

  Scenario: Overlapping members are synced once, not twice
    # Rows number in tree order: 1 LICENSE, 2 README.md, 3 src/main.go.
    Given that the file "LICENSE" has been changed locally
    And   that the file "README.md" has been changed locally
    And   that the file "src/main.go" has been changed locally
    When  I run "csync ./project user@host:/project" and respond with "1-3,2"
    Then  the reported sync count should be 3
    And   the file "LICENSE" should be identical between local and remote
    And   the file "README.md" should be identical between local and remote
    And   the file "src/main.go" should be identical between local and remote

  Scenario: An out-of-range member rejects the whole selection
    Given that the file "LICENSE" has been changed locally
    And   that the file "README.md" has been changed locally
    When  I run "csync ./project user@host:/project" and respond with "1-3"
    Then  csync should return a non-zero exit code
    And   the file "LICENSE" should still differ between local and remote
    And   the file "README.md" should still differ between local and remote

  Scenario: A reversed range is rejected
    # It is not silently reordered to "1-3".
    Given that the file "LICENSE" has been changed locally
    And   that the file "README.md" has been changed locally
    And   that the file "src/main.go" has been changed locally
    When  I run "csync ./project user@host:/project" and respond with "3-1"
    Then  csync should return a non-zero exit code
    And   the file "LICENSE" should still differ between local and remote
    And   the file "README.md" should still differ between local and remote
    And   the file "src/main.go" should still differ between local and remote

  Scenario: Whitespace around members and range operands is ignored
    # Rows number in tree order: 1 LICENSE, 2 README.md, 3 src/main.go, 4 src/parser.go.
    Given that the file "LICENSE" has been changed locally
    And   that the file "README.md" has been changed locally
    And   that the file "src/main.go" has been changed locally
    And   that the file "src/parser.go" has been changed locally
    When  I run "csync ./project user@host:/project" and respond with "1 - 2, 4"
    Then  the reported sync count should be 3
    And   the file "LICENSE" should be identical between local and remote
    And   the file "README.md" should be identical between local and remote
    And   the file "src/parser.go" should be identical between local and remote
    And   the file "src/main.go" should still differ between local and remote

  Scenario: A file removed on the source is reported as a deletion
    # No selection is made, so the deletion is reported but not applied.
    Given that the file "README.md" has been deleted locally
    When  I run "csync ./project user@host:/project"
    Then  the reported actions should be:
      | action | path      |
      | delete | README.md |
    And   the reported change count should be 1
    And   the file "README.md" should still exist on the remote

  Scenario: A selected deletion is applied to the destination
    # A nested file shows the removal reaches inside folders.
    Given that the file "src/parser.go" has been deleted locally
    When  I run "csync ./project user@host:/project" and respond with "a"
    Then  the reported sync count should be 1
    And   the file "src/parser.go" should not exist on the remote

  Scenario: Declining a deletion leaves the file on the destination
    Given that the file "README.md" has been deleted locally
    When  I run "csync ./project user@host:/project" and respond with "n"
    Then  the reported sync count should be 0
    And   the file "README.md" should still exist on the remote

  Scenario: A run mixing a transfer and a deletion applies and reports both
    # The summary calls out the removals, so "2 files total" doesn't read as two
    # transfers.
    Given that the file "src/adder.go" has been added locally
    And   that the file "README.md" has been deleted locally
    When  I run "csync ./project user@host:/project" and respond with "a"
    Then  the reported sync count should be 2
    And   the reported removed count should be 1
    And   the file "src/adder.go" should be identical between local and remote
    And   the file "README.md" should not exist on the remote

  Scenario: Selecting only the transfer leaves the deletion unapplied
    # Rows number in tree order: 1 adder.go, 2 README.md (the deletion).
    Given that the file "adder.go" has been added locally
    And   that the file "README.md" has been deleted locally
    When  I run "csync ./project user@host:/project" and respond with "1"
    Then  the reported sync count should be 1
    And   the file "adder.go" should be identical between local and remote
    And   the file "README.md" should still exist on the remote

  Scenario: A completed deletion leaves nothing to re-sync
    Given that the file "README.md" has been deleted locally
    When  I run "csync ./project user@host:/project" and respond with "a"
    And   I run "csync ./project user@host:/project" a second time
    Then  no actions should be reported
    And   the reported change count should be 0

  Scenario: A deletion candidate whose name holds a glob character is not offered
    # rsync filter rules treat *, ?, and [ as wildcards, so removing a[1].txt
    # could remove a1.txt instead. Until escaping lands, such files are not
    # offered for deletion.
    Given that the file "a[1].txt" has been added on the remote
    When  I run "csync ./project user@host:/project"
    Then  no actions should be reported
    And   the reported change count should be 0
    And   the file "a[1].txt" should still exist on the remote

  @wip
  Scenario: Pull direction — a remote-new file is brought down when selected
    Given that the file "notes.txt" has been added on the remote
    When  I run "csync user@host:/project ./project" and respond with "a"
    Then  the reported sync count should be 1
    And   the file "notes.txt" should be identical between local and remote

  @wip
  Scenario: A selected filename containing a newline transfers intact
    # A newline in a filename must not split it into two entries. See SECURITY.md.
    Given that a file whose name contains a newline has been added locally
    When  I run "csync ./project user@host:/project" and respond with "a"
    Then  the reported sync count should be 1
    And   that file should be identical between local and remote

  # ---------------------------------------------------------------------------
  # TODO: Additional scenarios for this feature, not yet drafted.
  # Each will become a real Scenario block as we drill into it.
  # ---------------------------------------------------------------------------
  #
  # - Non-interactive escape hatch: a future --all/-y flag skips the prompt and
  #   syncs everything. Tied to the tabled flag-parsing work; out of MVP scope.
