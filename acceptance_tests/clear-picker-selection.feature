@remote
Feature: Clear every selection in the picker

  In order to pick just a few changes out of many, I want to uncheck every
  row at once instead of one at a time.

  Background:
    Given a local directory containing these files:
      """
      src/main.go
      README.md
      """
    And   that the file "README.md" has been changed locally
    And   that the file "src/main.go" has been changed locally

  Scenario: Pressing "n" unchecks every row
    Given that I have run "csync ./project user@host:/project" in a terminal
    When  I press "n"
    Then  no row should be checked

  Scenario: Pressing "n" after unchecking one row unchecks the rest
    Given that I have run "csync ./project user@host:/project" in a terminal
    And   that I have unchecked the row for "README.md"
    When  I press "n"
    Then  the row for "src/main.go" should not be checked

  Scenario: Pressing "n" when nothing is checked leaves it that way
    # Unlike "a", "n" never toggles back to all-checked.
    Given that I have run "csync ./project user@host:/project" in a terminal
    And   that I have pressed "n"
    When  I press "n"
    Then  no row should be checked
