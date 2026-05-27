Feature: Invoke command

  In order to use csync against any local/remote pair without pre-configuration,
  I want to specify the source and destination as command-line arguments.

  Scenario: Push direction — local source, remote destination
    When I run "csync ./project user@host:/project"
    Then the reported source should be "./project"
    And  the reported destination should be "user@host:/project"
    And  the reported direction should be "push"

  # ---------------------------------------------------------------------------
  # TODO: Additional scenarios for this feature, not yet drafted.
  # Each will become a real Scenario block as we drill into it.
  # ---------------------------------------------------------------------------
  #
  # - Pull direction: `csync user@host:/project ./project` swaps the labels;
  #   the destination header is now the local path.
  #
  # - No arguments and no configuration: csync prints usage and exits non-zero.
  #
  # - One argument only: csync prints usage and exits non-zero.
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
