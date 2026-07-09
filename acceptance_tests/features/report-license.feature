Feature: Report license

  In order to comply with and inspect csync's license terms from the binary
  itself, I want csync to print its MIT license on request.

  Scenario: The --license flag prints the license and exits successfully
    When I run "csync --license"
    Then csync should return exit code 0
    And  the reported license should contain "MIT License"
    And  the reported license should contain "Copyright (c) 2026 David Passarelli"
    And  the reported license should contain "THE SOFTWARE IS PROVIDED"

  Scenario: --license short-circuits any operands
    When I run "csync --license ./project user@host:/project"
    Then csync should return exit code 0
    And  the reported license should contain "MIT License"
