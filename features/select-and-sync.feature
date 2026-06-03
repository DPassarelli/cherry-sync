# Scenarios are brought into the run one at a time as we implement them. Each
# not-yet-implemented scenario carries its own @wip tag (excluded via the
# "~@wip" filter in features_test.go); drop a scenario's tag when we drill in.
#
# @remote runs every scenario here over a fake SSH remote (RSYNC_RSH + a
# `fakehost:` operand), so rsync transfers in real sender/receiver mode and emits
# the `<f`/`>f` direction codes a true push/pull would. Local-to-local always
# itemizes `>`, which structurally hid a push-direction bug until this harness
# exposed it.
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
    And   that all of the files are identical between local and remote

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

  Scenario: A completed sync leaves nothing to re-sync
    # Idempotence guard: after csync transfers a set of changes, re-running the
    # same compare must report nothing left to do. A second run that still finds
    # differences means the transfer didn't fully reconcile the two sides — for
    # whatever reason — leaving the user re-syncing the same files indefinitely.
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
    # Triangulates the by-number selection: with "1" pinned to the first row
    # above, "2" must reach the second — proving the typed response is actually
    # read, not a hardcoded "always the first".
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

  @wip
  Scenario: An out-of-range number is rejected like an unrecognized response
    # Sibling to "An unrecognized response is rejected": with a single change in
    # the list, the only valid pick is 1, so "2" names no row. It gets the same
    # treatment as "wat" — reject, transfer nothing, non-zero exit.
    Given that the file "README.md" has been changed locally
    When  I run "csync ./project user@host:/project" and respond with "2"
    Then  csync should return a non-zero exit code
    And   the file "README.md" should still differ between local and remote

  @wip
  Scenario: Pull direction — a remote-new file is brought down when selected
    # The mirror of the push scenarios, source and destination swapped. A file
    # that exists only on the remote is created on the local side when selected,
    # exercising pull end to end (today only push is covered).
    Given that the file "notes.txt" has been added on the remote
    When  I run "csync user@host:/project ./project" and respond with "a"
    Then  the reported sync count should be 1
    And   the file "notes.txt" should be identical between local and remote

  @wip
  Scenario: A selected filename containing a newline transfers intact
    # SECURITY.md invariant: the --files-from list is NUL-delimited (--from0) so
    # a newline inside a filename can't split into a second entry. The new steps
    # keep the awkward byte out of the Gherkin — the create step picks a fixed
    # name containing a newline and remembers it; the assertion reads that same
    # name back. A split would fail to transfer the file intact, so the
    # identical-check is what proves the byte stayed inert.
    #
    # Heads up for drill-in: this also stresses the compare side. parseActions
    # splits rsync's --itemize-changes output on "\n", so a newline in a name
    # breaks diff parsing too, not just the transfer — expect to touch both.
    Given that a file whose name contains a newline has been added locally
    When  I run "csync ./project user@host:/project" and respond with "a"
    Then  the reported sync count should be 1
    And   that file should be identical between local and remote

  # ---------------------------------------------------------------------------
  # TODO: Additional scenarios for this feature, not yet drafted.
  # Each will become a real Scenario block as we drill into it.
  # ---------------------------------------------------------------------------
  #
  # - Range selection: a response like "1-3" or "1,3" selects multiple files by
  #   number. Decide the grammar (comma list, hyphen ranges, both) before
  #   drilling in.
  #
  # - Non-interactive escape hatch: a future --all/-y flag skips the prompt and
  #   syncs everything. Tied to the tabled flag-parsing work; out of MVP scope.
