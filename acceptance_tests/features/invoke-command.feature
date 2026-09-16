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

  Scenario: A local source written with a ~ home shortcut finds the files
    # Teeth for #71: rsync takes a literal "~" as a directory name, so an
    # unexpanded ~/project resolves against the CWD and the comparison finds
    # nothing (exit 23). csync expands it before rsync sees it. Drop the
    # expansion and no action is reported — red.
    Given a local directory in the home directory containing these files:
      """
      README.md
      """
    And   an empty remote directory
    When  I run "csync ~/project user@host:/project"
    Then  the reported actions should be:
      | action | path      |
      | create | README.md |

  Scenario: The expansion of a local ~ is disclosed
    # The expansion changes which directory the run works on, so it is named
    # beside the operand rather than applied silently.
    When I run "csync ~/project user@host:/project"
    Then csync should report that it rewrote "~/project"

  Scenario: A local ~user home shortcut is rejected
    # ~user names another user's home, which csync does not resolve. It is
    # rejected up front, as the remote side already rejects it, rather than
    # reaching rsync as a literal directory name.
    When I run "csync ~deploy/project user@host:/project"
    Then csync should return a non-zero exit code
    And  the reported error should mention "~"
    And  no run log should have been written

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
