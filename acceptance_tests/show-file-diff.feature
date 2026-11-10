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

  Scenario: The diff is titled with the file it shows
    Given that the file "README.md" has been changed locally
    And   that I have run "csync ./project user@host:/project" in a terminal
    When  I press "d" on the row for "README.md"
    Then  the diff overlay's title should read "Diff of README.md"

  Scenario: The diff names each side by its operand
    Given that the file "README.md" has been changed locally
    And   that I have run "csync ./project user@host:/project" in a terminal
    When  I press "d" on the row for "README.md"
    Then  the diff overlay's header should read:
      """
      - Destination: user@host:/project
      + Source:      ./project
      """

  Scenario: The diff is drawn inside a box
    Given that the file "README.md" has been changed locally
    And   that I have run "csync ./project user@host:/project" in a terminal
    When  I press "d" on the row for "README.md"
    Then  the diff overlay should be drawn inside a box

  Scenario: The box has a blank row above it
    Given that the file "README.md" has been changed locally
    And   that I have run "csync ./project user@host:/project" in a terminal
    When  I press "d" on the row for "README.md"
    Then  the diff overlay's box should have a blank row above it

  Scenario: A diff taller than the screen shows a scrollbar
    Given that the file "README.md" has been changed locally so that its diff is taller than the terminal
    And   that I have run "csync ./project user@host:/project" in a terminal
    When  I press "d" on the row for "README.md"
    Then  the diff overlay's scrollbar should be at the top

  Scenario: The scrollbar's thumb is bold
    # The box is faint, so the thumb is drawn at full brightness and bold to stand
    # out from it. The emulator records bold but not faint, so bold is what's checked.
    Given that the file "README.md" has been changed locally so that its diff is taller than the terminal
    And   that I have run "csync ./project user@host:/project" in a terminal
    When  I press "d" on the row for "README.md"
    Then  the diff overlay's scrollbar thumb should be bold

  Scenario: Scrolling to the end moves the scrollbar to the bottom
    Given that the file "README.md" has been changed locally so that its diff is taller than the terminal
    And   that I have run "csync ./project user@host:/project" in a terminal
    And   that I have pressed "d" on the row for "README.md"
    When  I press "down" more times than the diff has lines
    Then  the diff overlay's scrollbar should be at the bottom

  Scenario: A diff that fits shows no scrollbar
    Given that the file "README.md" has been changed locally
    And   that I have run "csync ./project user@host:/project" in a terminal
    When  I press "d" on the row for "README.md"
    Then  the diff overlay should show no scrollbar

  Scenario: The title and header stay put while the diff scrolls
    Given that the file "README.md" has been changed locally so that its diff is taller than the terminal
    And   that I have run "csync ./project user@host:/project" in a terminal
    And   that I have pressed "d" on the row for "README.md"
    When  I press "down" more times than the diff has lines
    Then  the diff overlay's title should read "Diff of README.md"

  Scenario Outline: The <side> is shown in <color>, in the header and in the diff
    Given that the file "README.md" has been changed locally from "old line" to "new line"
    And   that I have run "csync ./project user@host:/project" in a terminal
    When  I press "d" on the row for "README.md"
    Then  the lines beginning "<header>" and "<line>" should be <color>

    Examples:
      | side        | color | header         | line      |
      | destination | red   | - Destination: | -old line |
      | source      | green | + Source:      | +new line |

  Scenario: A file only on the source has no diff
    Given that the file "src/adder.go" has been added locally
    And   that I have run "csync ./project user@host:/project" in a terminal
    When  I press "d" on the row for "src/adder.go"
    Then  the picker should note "cannot diff: dest does not exist!"

  Scenario: A file only on the destination has no diff
    Given that the file "README.md" has been deleted locally
    And   that I have run "csync ./project user@host:/project" in a terminal
    When  I press "d" on the row for "README.md"
    Then  the picker should note "cannot diff: src does not exist!"

  Scenario Outline: A copy over 200 KiB is too large to diff
    # The local copy is checked first, so when both are too large it is the one named.
    Given that <which> of "README.md" has been changed to 204,801 bytes
    And   that I have run "<command>" in a terminal
    When  I press "d" on the row for "README.md"
    Then  the picker should note "<note>"

    Examples:
      | which           | command                            | note                            |
      | the local copy  | csync ./project user@host:/project | cannot diff: src file too big!  |
      | the remote copy | csync ./project user@host:/project | cannot diff: dest file too big! |
      | both copies     | csync ./project user@host:/project | cannot diff: src file too big!  |
      | the local copy  | csync user@host:/project ./project | cannot diff: dest file too big! |
      | both copies     | csync user@host:/project ./project | cannot diff: dest file too big! |

  Scenario: A file of exactly 200 KiB still gets a diff
    Given that the local copy of "README.md" has been changed to 204,800 bytes
    And   that I have run "csync ./project user@host:/project" in a terminal
    When  I press "d" on the row for "README.md"
    Then  the diff overlay's title should read "Diff of README.md"

  Scenario Outline: A copy with a NUL byte is treated as binary
    Given that <which> of "README.md" has been changed to contain a NUL byte
    And   that I have run "csync ./project user@host:/project" in a terminal
    When  I press "d" on the row for "README.md"
    Then  the picker should note "cannot diff: binary files!"

    Examples:
      | which           |
      | the local copy  |
      | the remote copy |
      | both copies     |

  Scenario Outline: A local copy that can't be diffed spares the remote
    Given that the local copy of "README.md" has been changed to <content>
    And   a remote that records each file it sends
    And   that I have run "csync ./project user@host:/project" in a terminal
    When  I press "d" on the row for "README.md"
    Then  the remote should not have been asked for a copy

    Examples:
      | content            |
      | 204,801 bytes      |
      | contain a NUL byte |

  Scenario: A note about a file that can't be diffed is bold
    Given that the file "adder.go" has been added locally
    And   that I have run "csync ./project user@host:/project" in a terminal
    When  I press "d" on the row for "adder.go"
    Then  the note "cannot diff: dest does not exist!" should be bold

  @wip
  Scenario: A file whose contents match shows no empty diff
    # Not reachable as written: a file that was only touched isn't reported as a
    # change, so it has no row. Identical contents can only arise if the copies come
    # to match after the comparison.
    Given that the file "README.md" has been touched locally
    And   that I have run "csync ./project user@host:/project" in a terminal
    When  I press "d" on the row for "README.md"
    Then  the picker should note "cannot diff: contents are identical!"

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
    Then  the picker's note should contain "fetching diff"

  Scenario: A failed fetch is reported inside the diff
    Given that the file "README.md" has been changed locally
    And   a remote that answers the comparison but fails to send files
    And   that I have run "csync ./project user@host:/project" in a terminal
    When  I press "d" on the row for "README.md"
    Then  the diff overlay should begin with "couldn't fetch for diff:"

  Scenario: A failed fetch still says which file it was for
    Given that the file "README.md" has been changed locally
    And   a remote that answers the comparison but fails to send files
    And   that I have run "csync ./project user@host:/project" in a terminal
    When  I press "d" on the row for "README.md"
    Then  the diff overlay's title should read "Diff of README.md"

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

  Scenario: The picker's hint line leaves the diff to a line of its own
    Given that the file "README.md" has been changed locally
    When  I run "csync ./project user@host:/project" in a terminal
    Then  the hint line should read "↑/↓ move · space toggle · a all/none · enter sync · ctrl-c cancel"

  Scenario: The diff hint says which files can be diffed
    Given that the file "README.md" has been changed locally
    When  I run "csync ./project user@host:/project" in a terminal
    Then  the diff hint should read "d diff (text files ≤ 200 KiB only)"

  Scenario: The diff's hint line offers scrolling and going back
    Given that the file "README.md" has been changed locally so that its diff is taller than the terminal
    And   that I have run "csync ./project user@host:/project" in a terminal
    When  I press "d" on the row for "README.md"
    Then  the hint line should read "↑/↓ scroll · esc back"

  Scenario: A diff that fits offers only going back
    Given that the file "README.md" has been changed locally
    And   that I have run "csync ./project user@host:/project" in a terminal
    When  I press "d" on the row for "README.md"
    Then  the hint line should read "esc back"
