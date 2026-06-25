# Scenarios are brought into the run one at a time as we implement them. Each
# not-yet-implemented scenario carries its own @wip tag (excluded via the
# "~@wip" filter in features_test.go); drop a scenario's tag when we drill in.
#
# @remote runs every scenario over a fake SSH remote, so the push/pull scenarios
# transfer in real sender/receiver mode (see select-and-sync.feature for why
# local-to-local would hide direction bugs). The remote operand now also arrives
# from ./.csync.toml rather than argv, so the @remote harness must rewrite the
# configured remote the same way it rewrites an argv operand.
#
# New steps this feature will need at drill-in:
#   - a ".csync.toml" in the project directory containing: <docstring>
#   - I run "csync push|pull" from the project directory  (chdir so cwd-only
#     discovery finds the dotfile)
#   - the reported error should mention "<text>"
@remote
Feature: Saved sync targets

  In order to avoid retyping a remote I sync with often, I want to save it once
  in the project's .csync.toml and refer to it with `csync push` / `csync pull`.

  @wip
  Scenario: Push resolves the saved remote as the destination
    # Teeth: push must read `remote` from ./.csync.toml and use it as the
    # destination, with the project directory (cwd) as the source. README is
    # changed locally; after `csync push` it must match the remote. Point the
    # config at the wrong side (or ignore it) and README stays divergent — red.
    Given a local directory containing these files:
      """
      src/main.go
      README.md
      """
    And   that all of the files are identical between local and remote
    And   a ".csync.toml" in the project directory containing:
      """
      remote = "user@host:/project"
      """
    And   that the file "README.md" has been changed locally
    When  I run "csync push" from the project directory and respond with "a"
    Then  the reported sync count should be 1
    And   the file "README.md" should be identical between local and remote

  @wip
  Scenario: Pull resolves the saved remote as the source
    # The mirror of push: pull must use the configured `remote` as the SOURCE and
    # the project directory as the destination. A file that exists only on the
    # remote is brought down. Swap source/destination resolution and this file
    # never lands locally — red.
    Given a local directory containing these files:
      """
      src/main.go
      README.md
      """
    And   that all of the files are identical between local and remote
    And   a ".csync.toml" in the project directory containing:
      """
      remote = "user@host:/project"
      """
    And   that the file "notes.txt" has been added on the remote
    When  I run "csync pull" from the project directory and respond with "a"
    Then  the reported sync count should be 1
    And   the file "notes.txt" should be identical between local and remote

  @wip
  Scenario: Push reports the resolved source and destination
    # Resolution proof independent of any transfer: with nothing to sync, csync
    # prints the resolved operands and exits without prompting. The destination
    # must be the configured remote; the source is the project directory itself,
    # shown as ".". (The "." display is a v1 choice — see TODO below.)
    Given a local directory containing these files:
      """
      README.md
      """
    And   that all of the files are identical between local and remote
    And   a ".csync.toml" in the project directory containing:
      """
      remote = "user@host:/project"
      """
    When  I run "csync push" from the project directory
    Then  the reported source should be "."
    And   the reported destination should be "user@host:/project"
    And   the reported message should begin with "No changes"

  @wip
  Scenario: A missing .csync.toml fails loudly and transfers nothing
    # No silent fallback to a default. With no dotfile in the project directory,
    # `csync push` must error and exit non-zero BEFORE contacting rsync — the
    # changed README must still differ, proving no transfer was attempted.
    Given a local directory containing these files:
      """
      README.md
      """
    And   that all of the files are identical between local and remote
    And   that the file "README.md" has been changed locally
    When  I run "csync push" from the project directory
    Then  csync should return a non-zero exit code
    And   the reported error should mention ".csync.toml"
    And   the file "README.md" should still differ between local and remote

  @wip
  Scenario: A .csync.toml with no remote key is rejected
    # The file exists but defines nothing usable. Rejected like a missing file:
    # non-zero exit, no transfer. Distinguishes "file present" from "remote
    # present" — an empty config is not an implicit default.
    Given a local directory containing these files:
      """
      README.md
      """
    And   that all of the files are identical between local and remote
    And   a ".csync.toml" in the project directory containing:
      """
      # no remote defined yet
      """
    And   that the file "README.md" has been changed locally
    When  I run "csync push" from the project directory
    Then  csync should return a non-zero exit code
    And   the file "README.md" should still differ between local and remote

  @wip
  Scenario: A .csync.toml with an empty remote value is rejected
    # SECURITY.md: a config-sourced path gets the SAME validation as a CLI path.
    # An empty `remote` becomes the same footgun an empty argv path would (an
    # operand of ""), so it is rejected, not passed to rsync. This is the guard
    # that config is not a back door around path validation.
    Given a local directory containing these files:
      """
      README.md
      """
    And   that all of the files are identical between local and remote
    And   a ".csync.toml" in the project directory containing:
      """
      remote = ""
      """
    And   that the file "README.md" has been changed locally
    When  I run "csync push" from the project directory
    Then  csync should return a non-zero exit code
    And   the file "README.md" should still differ between local and remote

  @wip
  Scenario: Malformed TOML is rejected with a clear error
    # A syntactically broken dotfile must fail with a parse error, not be
    # silently ignored or half-read. Non-zero exit, no transfer.
    Given a local directory containing these files:
      """
      README.md
      """
    And   that all of the files are identical between local and remote
    And   a ".csync.toml" in the project directory containing:
      """
      remote = "user@host:/project
      """
    And   that the file "README.md" has been changed locally
    When  I run "csync push" from the project directory
    Then  csync should return a non-zero exit code
    And   the file "README.md" should still differ between local and remote

  # ---------------------------------------------------------------------------
  # TODO: Additional scenarios for this feature, not yet drafted.
  # Each will become a real Scenario block as we drill into it.
  # ---------------------------------------------------------------------------
  #
  # - Source display for push: v1 shows the local operand as ".". Decide whether
  #   to show the resolved absolute cwd instead (clearer in logs, longer line).
  #
  # - Discovery is cwd-only in v1: running `csync push` from a SUBDIRECTORY of a
  #   project whose .csync.toml sits at the root must NOT find it (it errors).
  #   When walk-up discovery lands, this flips to "found from a subdirectory".
  #
  # - `push`/`pull` reject extra positional arguments (`csync push ./x`): the
  #   verb forms take no operands; surface a usage error.
  #
  # - A remote value that looks like an rsync option (e.g. "-e...") reaches rsync
  #   as an inert path via the `--` guard, exactly as the argv case already does
  #   in compare-directories.feature. SECURITY regression guard for the config
  #   path source.
