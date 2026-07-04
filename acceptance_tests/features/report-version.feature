Feature: Report version

  In order to tell which build of csync I'm running when comparing behavior or
  filing a report, I want csync to print its version on request.

  Scenario: The --version flag prints the version and exits successfully
    When I run "csync --version"
    Then csync should return exit code 0
    And  the reported version should be "cherry-sync v0.0.0-test"

  Scenario: --version short-circuits any operands
    When I run "csync --version ./project user@host:/project"
    Then csync should return exit code 0
    And  the reported version should be "cherry-sync v0.0.0-test"

  # ---------------------------------------------------------------------------
  # TODO: Additional scenarios for this feature, not yet drafted.
  # ---------------------------------------------------------------------------
  #
  # - The version at the top of the interactive header. Rendering the banner is
  #   TTY-gated, so it is proven by a view-package unit test rather than a godog
  #   scenario (the suite runs csync non-interactively); revisit once the PTY
  #   harness lands.
  #
  # - A "-v" short alias and/or a bare "version" verb: deliberately not added —
  #   "-v" is held in reserve for a future verbose flag.
