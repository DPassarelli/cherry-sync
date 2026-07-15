Feature: Show help

  In order to learn how to invoke csync without leaving the terminal, I want a
  help flag that prints a usage summary to stdout and exits successfully.

  Scenario: The --help flag describes csync on stdout and exits successfully
    When I run "csync --help"
    Then csync should return exit code 0
    And  the help text should contain "cherry-sync"
    And  the help text should contain "An interactive rsync wrapper"

  Scenario: Help lists the commands and flags
    When I run "csync --help"
    Then the help text should contain "push"
    And  the help text should contain "pull"
    And  the help text should contain "--license"

  Scenario: Help shows usage examples
    When I run "csync --help"
    Then the help text should contain "EXAMPLES"

  Scenario: The -h alias behaves like --help
    When I run "csync -h"
    Then csync should return exit code 0
    And  the help text should contain "cherry-sync"

  Scenario: --help short-circuits any operands
    When I run "csync --help ./project user@host:/project"
    Then csync should return exit code 0
    And  the help text should contain "cherry-sync"
