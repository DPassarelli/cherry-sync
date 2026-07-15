Feature: Invoke command

  In order to use csync against any local/remote pair without pre-configuration,
  I want to specify the source and destination as command-line arguments.

  Scenario: Push direction — local source, remote destination
    When I run "csync ./project user@host:/project"
    Then the reported source should be "./project"
    And  the reported destination should be "user@host:/project"

  Scenario: Pull direction — remote source, local destination
    When I run "csync user@host:/project ./project"
    Then the reported source should be "user@host:/project"
    And  the reported destination should be "./project"

  Scenario: No arguments — report the problem and exit non-zero
    When I run "csync"
    Then csync should return exit code 2
    And  the reported error should mention "a source and a destination"
    And  the reported error should mention "csync --help"

  Scenario: One path only — report that both are required
    When I run "csync ./project"
    Then csync should return exit code 2
    And  the reported error should mention "a source and a destination"

  Scenario: A mistyped command is reported as such
    When I run "csync pill"
    Then csync should return exit code 2
    And  the reported error should mention "'pill' is not a command"
    And  the reported error should mention "pull"

  Scenario: Empty path argument — report the empty operand and exit non-zero
    When I run "csync <empty> user@host:/project"
    Then csync should return exit code 2
    And  the reported error should mention "source path is empty"

  # ---------------------------------------------------------------------------
  # TODO: Additional scenarios for this feature, not yet drafted.
  # Each will become a real Scenario block as we drill into it.
  # ---------------------------------------------------------------------------
  #
  # - Three or more arguments: rejected in v0.1 (we are not supporting multiple
  #   sources yet).
  #
  # - Both paths local (no `user@host:` prefix on either): decide if v0.1
  #   supports this or rejects it with a message pointing to plain rsync.
  #
  # - Both paths remote: rejected — csync uses local SSH transport, no
  #   daemon-mode support.
  #
  # - Arguments override configuration: when csync is configured with one pair
  #   but invoked with explicit arguments, the arguments win.
  #
  # - Trailing-slash semantics: decide whether csync preserves rsync's classic
  #   `./foo` vs `./foo/` distinction (pass-through) or normalizes it.
  #
  # - Input-handling / injection concerns (leading-dash paths, empty paths,
  #   newline-in-filename, etc.) live in SECURITY.md — see its concern catalog.
