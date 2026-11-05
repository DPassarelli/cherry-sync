@remote
Feature: Show a file's diff from the picker

  In order to decide whether a change is worth syncing, I want to see what
  differs inside a file without leaving the picker.

  Background:
    Given a local directory containing these files:
      """
      src/main.go
      README.md
      """

  Scenario: Pressing "d" on a changed file shows what changed
    Given that the file "README.md" has been changed locally from "old line" to "new line"
    And   that I have run "csync ./project user@host:/project" in a terminal
    When  I press "d" on the row for "README.md"
    Then  the diff overlay should include:
      """
      -old line
      +new line
      """

  Scenario: The diff labels each side by its role in the sync
    Given that the file "README.md" has been changed locally
    And   that I have run "csync ./project user@host:/project" in a terminal
    When  I press "d" on the row for "README.md"
    Then  the diff overlay should begin with:
      """
      --- destination: README.md
      +++ source: README.md
      """

  Scenario: A file only on the source has no diff
    Given that the file "src/adder.go" has been added locally
    And   that I have run "csync ./project user@host:/project" in a terminal
    When  I press "d" on the row for "src/adder.go"
    Then  the picker should show a notice containing "will be created on the destination"

  Scenario: A file only on the destination has no diff
    Given that the file "README.md" has been deleted locally
    And   that I have run "csync ./project user@host:/project" in a terminal
    When  I press "d" on the row for "README.md"
    Then  the picker should show a notice containing "will be deleted from the destination"

  Scenario Outline: A copy over 200 KiB is too large to diff
    Given that <which> of "README.md" has been changed to 204,801 bytes
    And   that I have run "csync ./project user@host:/project" in a terminal
    When  I press "d" on the row for "README.md"
    Then  the picker should show a notice containing "<named> over the 200 KiB limit"

    Examples:
      | which           | named               |
      | the local copy  | source copy is      |
      | the remote copy | destination copy is |
      | both copies     | both copies are     |

  Scenario: A file of exactly 200 KiB still gets a diff
    Given that the local copy of "README.md" has been changed to 204,800 bytes
    And   that I have run "csync ./project user@host:/project" in a terminal
    When  I press "d" on the row for "README.md"
    Then  the diff overlay should begin with:
      """
      --- destination: README.md
      +++ source: README.md
      """

  Scenario Outline: A copy with a NUL byte is treated as binary
    Given that <which> of "README.md" has been changed to contain a NUL byte
    And   that I have run "csync ./project user@host:/project" in a terminal
    When  I press "d" on the row for "README.md"
    Then  the picker should show a notice containing "<named> binary, and binary files are not diffed"

    Examples:
      | which           | named               |
      | the local copy  | source copy is      |
      | the remote copy | destination copy is |
      | both copies     | both copies are     |

  @wip
  Scenario: A file whose contents match shows no empty diff
    # Not reachable as written: a file that was only touched isn't reported as a
    # change, so it has no row. Identical contents can only arise if the copies come
    # to match after the comparison.
    Given that the file "README.md" has been touched locally
    And   that I have run "csync ./project user@host:/project" in a terminal
    When  I press "d" on the row for "README.md"
    Then  the picker should show a notice containing "contents are identical"

  Scenario: The next keypress clears a notice
    Given that the file "adder.go" has been added locally
    And   that I have run "csync ./project user@host:/project" in a terminal
    And   that I have pressed "d" on the row for "adder.go"
    When  I press "down"
    Then  the notice should no longer be visible

  Scenario: The keypress that clears a notice still takes effect
    # Rows are in tree order: adder.go, then README.md.
    Given that the file "adder.go" has been added locally
    And   that the file "README.md" has been changed locally
    And   that I have run "csync ./project user@host:/project" in a terminal
    And   that I have pressed "d" on the row for "adder.go"
    When  I press "down"
    Then  the cursor should be on the row for "README.md"

  Scenario: Scrolling the diff reveals its last line
    Given that the file "README.md" has been changed locally so that its diff is taller than the terminal
    And   that I have run "csync ./project user@host:/project" in a terminal
    And   that I have pressed "d" on the row for "README.md"
    And   that the diff overlay does not show its last line
    When  I press "down" more times than the diff has lines
    Then  the diff overlay should show its last line

  Scenario: Scrolling the diff stops at its last line
    # Had scrolling run past the end, one "up" would leave the last line on screen.
    Given that the file "README.md" has been changed locally so that its diff is taller than the terminal
    And   that I have run "csync ./project user@host:/project" in a terminal
    And   that I have pressed "d" on the row for "README.md"
    And   that I have pressed "down" more times than the diff has lines
    When  I press "up"
    Then  the diff overlay should not show its last line

  Scenario: The escape key closes the diff
    Given that the file "README.md" has been changed locally
    And   that I have run "csync ./project user@host:/project" in a terminal
    And   that I have pressed "d" on the row for "README.md"
    When  I press "esc"
    Then  the diff overlay should no longer be visible

  Scenario: Closing the diff brings back what was above the picker
    Given that the file "README.md" has been changed locally
    And   that I have run "csync ./project user@host:/project" in a terminal
    And   that I have pressed "d" on the row for "README.md"
    When  I press "esc"
    Then  the source and destination should be on screen

  Scenario Outline: Pressing "<key>" leaves the diff open
    Given that the file "README.md" has been changed locally
    And   that I have run "csync ./project user@host:/project" in a terminal
    And   that I have pressed "d" on the row for "README.md"
    When  I press "<key>"
    Then  the diff overlay should still be open

    Examples:
      | key   |
      | enter |
      | space |
      | a     |
      | d     |

  Scenario: Pressing "enter" in the diff transfers nothing
    Given that the file "README.md" has been changed locally
    And   that I have run "csync ./project user@host:/project" in a terminal
    And   that I have pressed "d" on the row for "README.md"
    When  I press "enter"
    Then  no files should have been transferred

  Scenario Outline: Pressing "<key>" in the diff leaves the checkboxes alone
    # The picker starts all-checked, and the checkboxes are hidden behind the overlay.
    Given that the file "README.md" has been changed locally
    And   that I have run "csync ./project user@host:/project" in a terminal
    And   that I have pressed "d" on the row for "README.md"
    And   that I have pressed "<key>"
    When  I press "esc"
    Then  the row for "README.md" should be checked

    Examples:
      | key   |
      | space |
      | a     |

  Scenario: Ctrl-C in the diff cancels the run
    Given that the file "README.md" has been changed locally
    And   that I have run "csync ./project user@host:/project" in a terminal
    And   that I have pressed "d" on the row for "README.md"
    When  I press "ctrl-c"
    Then  csync should have exited

  Scenario: A slow fetch shows that the diff is on its way
    Given that the file "README.md" has been changed locally
    And   a remote that answers the comparison but is slow to send files
    And   that I have run "csync ./project user@host:/project" in a terminal
    When  I press "d" on the row for "README.md"
    Then  the picker should show a notice containing "fetching diff"

  Scenario: A failed fetch is reported inside the diff
    Given that the file "README.md" has been changed locally
    And   a remote that answers the comparison but fails to send files
    And   that I have run "csync ./project user@host:/project" in a terminal
    When  I press "d" on the row for "README.md"
    Then  the diff overlay should begin with "couldn't fetch for diff:"

  Scenario: A failed fetch repeats what rsync said
    Given that the file "README.md" has been changed locally
    And   a remote that answers the comparison but fails to send files
    And   that I have run "csync ./project user@host:/project" in a terminal
    When  I press "d" on the row for "README.md"
    Then  the diff overlay should include what the remote said about the failure

  Scenario: A long failure message wraps instead of being cut off
    Given that the file "README.md" has been changed locally
    And   a remote that fails to send files with a message wider than the terminal
    And   that I have run "csync ./project user@host:/project" in a terminal
    When  I press "d" on the row for "README.md"
    Then  the diff overlay should include all of what the remote said

  Scenario: A long diff line is shown in full
    Given that the file "README.md" has been changed locally to a single line wider than the terminal
    And   that I have run "csync ./project user@host:/project" in a terminal
    When  I press "d" on the row for "README.md"
    Then  the diff overlay should include all of that line

  Scenario: A wrapped line continues behind the wrap indicator
    Given that the file "README.md" has been changed locally to a single line wider than the terminal
    And   that I have run "csync ./project user@host:/project" in a terminal
    When  I press "d" on the row for "README.md"
    Then  the row after that line's first row should begin with "↪"

  Scenario: Each tab is shown as a spaced arrow
    Given that the file "README.md" has been changed locally to the line "a<tab><tab>b"
    And   that I have run "csync ./project user@host:/project" in a terminal
    When  I press "d" on the row for "README.md"
    Then  the diff overlay should include the line "+a → →b"

  Scenario: The picker's hint line offers the diff
    Given that the file "README.md" has been changed locally
    When  I run "csync ./project user@host:/project" in a terminal
    Then  the hint line should read "↑/↓ move · space toggle · a all/none · d diff · enter sync · ctrl-c cancel"

  Scenario: The diff's hint line offers scrolling and going back
    Given that the file "README.md" has been changed locally
    And   that I have run "csync ./project user@host:/project" in a terminal
    When  I press "d" on the row for "README.md"
    Then  the hint line should read "↑/↓ scroll · esc back"
