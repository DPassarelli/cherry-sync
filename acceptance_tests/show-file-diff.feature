# Every scenario here is @wip until the acceptance suite can drive the picker
# through a real terminal; the picker's unit tests gate this behavior meanwhile.
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

  @wip
  Scenario: Pressing "d" on a changed file shows its diff
    Given that the file "README.md" has been changed locally
    And   that I have run "csync ./project user@host:/project" in a terminal
    When  I press "d" on the row for "README.md"
    Then  the diff overlay should begin with:
      """
      --- destination: README.md
      +++ source: README.md
      """

  @wip
  Scenario: A file on only one side has no diff
    Given that the file "src/adder.go" has been added locally
    And   that I have run "csync ./project user@host:/project" in a terminal
    When  I press "d" on the row for "src/adder.go"
    Then  the picker should show the notice "no diff: file exists on one side only"

  @wip
  Scenario: A file over 200 KiB is too large to diff
    Given that the file "README.md" has been changed locally to 204,801 bytes
    And   that I have run "csync ./project user@host:/project" in a terminal
    When  I press "d" on the row for "README.md"
    Then  the picker should show the notice "too large to diff"

  @wip
  Scenario: A file with a NUL byte is treated as binary
    Given that the file "README.md" has been changed locally to contain a NUL byte
    And   that I have run "csync ./project user@host:/project" in a terminal
    When  I press "d" on the row for "README.md"
    Then  the picker should show the notice "binary files differ"

  @wip
  Scenario: A file whose contents match shows no empty diff
    Given that the file "README.md" has been touched locally
    And   that I have run "csync ./project user@host:/project" in a terminal
    When  I press "d" on the row for "README.md"
    Then  the picker should show the notice "contents identical (only the timestamp differs)"

  @wip
  Scenario: The next keypress clears a notice and still takes effect
    Given that the file "src/adder.go" has been added locally
    And   that the file "README.md" has been changed locally
    And   that I have run "csync ./project user@host:/project" in a terminal
    And   that I have pressed "d" on the row for "src/adder.go"
    When  I press "down"
    Then  the picker should show no notice
    And   the cursor should be on the row for "README.md"

  @wip
  Scenario: The diff scrolls and stops at its end
    Given that the file "README.md" has been changed locally on every line of 200
    And   that I have run "csync ./project user@host:/project" in a terminal
    And   that I have pressed "d" on the row for "README.md"
    When  I press "down" 500 times
    Then  the diff overlay should show its last line

  @wip
  Scenario: Closing the diff returns to the picker unchanged
    Given that the file "README.md" has been changed locally
    And   that the file "src/adder.go" has been added locally
    And   that I have run "csync ./project user@host:/project" in a terminal
    And   that I have pressed "space" on the row for "src/adder.go"
    And   that I have pressed "d" on the row for "README.md"
    When  I press "esc"
    Then  the cursor should be on the row for "README.md"
    And   the row for "src/adder.go" should be unchecked

  @wip
  Scenario: Picker keys do nothing while the diff is open
    Given that the file "README.md" has been changed locally
    And   that I have run "csync ./project user@host:/project" in a terminal
    And   that I have pressed "d" on the row for "README.md"
    When  I press "enter"
    Then  the diff overlay should still be open
    And   no files should have been transferred

  @wip
  Scenario: Ctrl-C in the diff cancels the run
    Given that the file "README.md" has been changed locally
    And   that I have run "csync ./project user@host:/project" in a terminal
    And   that I have pressed "d" on the row for "README.md"
    When  I press "ctrl-c"
    Then  the files should end up:
      | path      | state       |
      | README.md | out of sync |

  @wip
  Scenario: A failed fetch is reported inside the diff
    Given that the file "README.md" has been changed locally
    And   that I have run "csync ./project user@host:/project" in a terminal
    And   that the remote has become unreachable
    When  I press "d" on the row for "README.md"
    Then  the diff overlay should begin with "couldn't fetch for diff:"

  @wip
  Scenario: The picker's hint line offers the diff
    Given that the file "README.md" has been changed locally
    When  I run "csync ./project user@host:/project" in a terminal
    Then  the hint line should read "↑/↓ move · space toggle · a all/none · d diff · enter sync · ctrl-c cancel"

  @wip
  Scenario: The diff's hint line offers scrolling and going back
    Given that the file "README.md" has been changed locally
    And   that I have run "csync ./project user@host:/project" in a terminal
    When  I press "d" on the row for "README.md"
    Then  the hint line should read "↑/↓ scroll · esc back"
