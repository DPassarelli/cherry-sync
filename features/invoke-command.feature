Feature: Invoke command

  In order to use csync against any local/remote pair without pre-configuration,
  I want to specify the source and destination as command-line arguments.

  Scenario: Push direction — local source, remote destination
    When I run "csync ./project user@host:/project"
    Then the reported source should be "./project"
    And  the reported destination should be "user@host:/project"

  Scenario: No arguments — show usage and exit non-zero
    When I run "csync"
    Then csync should return exit code 2
    And  the reported usage should begin with "usage: csync"

  Scenario: One argument — show usage and exit non-zero
    When I run "csync ./project"
    Then csync should return exit code 2
    And  the reported usage should begin with "usage: csync"

  # ---------------------------------------------------------------------------
  # TODO: Additional scenarios for this feature, not yet drafted.
  # Each will become a real Scenario block as we drill into it.
  # ---------------------------------------------------------------------------
  #
  # - Pull direction: `csync user@host:/project ./project` swaps the labels;
  #   the destination header is now the local path.
  #
  # - Usage-message content: once we settle what the rest of the message
  #   should include (synopsis line, brief description, pointer to `--help`),
  #   add scenarios asserting those fields beyond the `usage: csync` prefix.
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
