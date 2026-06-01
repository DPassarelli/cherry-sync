# Scenarios are brought into the run one at a time as we implement them. Each
# not-yet-implemented scenario carries its own @wip tag (excluded via the
# "~@wip" filter in features_test.go); drop a scenario's tag when we drill in.
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

  # ---------------------------------------------------------------------------
  # TODO: Additional scenarios for this feature, not yet drafted.
  # Each will become a real Scenario block as we drill into it.
  # ---------------------------------------------------------------------------
  #
  # - Range selection: a response like "1-3" or "1,3" selects multiple files by
  #   number. Decide the grammar (comma list, hyphen ranges, both) before
  #   drilling in.
  #
  # - Out-of-range number: a response referencing a number with no matching
  #   row is rejected the same way an unrecognized response is.
  #
  # - Pull direction: selecting a file new on the remote brings it down to the
  #   local side. Mirror of the push scenarios above.
  #
  # - Filenames with spaces / newlines: a selected path with awkward bytes is
  #   transferred intact (exercises the NUL-delimited --files-from invariant in
  #   SECURITY.md).
  #
  # - Non-interactive escape hatch: a future --all/-y flag skips the prompt and
  #   syncs everything. Tied to the tabled flag-parsing work; out of MVP scope.
