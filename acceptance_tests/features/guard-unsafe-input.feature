# @remote scenarios run over a fake SSH remote, so a path read from .csync.toml
# reaches rsync the way it would against a real host.
Feature: Treat every path as a literal path

  In order to sync safely whatever my files and paths are called, I want csync
  to hand every path to rsync as a literal path, and to refuse one it cannot
  handle safely, rather than let it change what rsync does.

  Scenario: A source that looks like an rsync option is treated as a path
    # A path that starts with "-" must reach rsync as a path, never as an
    # option (-e would run a remote shell). See SECURITY.md.
    When  I run "csync -e ./project"
    Then  csync should return a non-zero exit code

  @remote
  Scenario: A configured remote that looks like an rsync option is treated as a path
    # A path read from config must not get around the guard on option-looking
    # paths. See SECURITY.md.
    Given a local directory containing these files:
      """
      README.md
      """
    And   a ".csync.toml" in the project directory containing:
      """
      remote = "-e"
      """
    When  I run "csync pull" from the project directory
    Then  csync should return a non-zero exit code

  Scenario: Empty path argument — report the empty operand and exit non-zero
    When I run "csync <empty> user@host:/project"
    Then csync should return exit code 2
    And  the reported error should mention "source path is empty"

  @remote
  Scenario: A .csync.toml with an empty remote value is rejected
    # A path from config gets the same validation as one from the command line.
    # See SECURITY.md.
    Given a local directory containing these files:
      """
      README.md
      """
    And   a ".csync.toml" in the project directory containing:
      """
      remote = ""
      """
    And   that the file "README.md" has been changed locally
    When  I run "csync push" from the project directory
    Then  csync should return a non-zero exit code
    And   the reported error should mention ".csync.toml"
    And   the file "README.md" should still differ between local and remote

  @remote
  Scenario: A deletion candidate whose name holds a glob character is not offered
    # rsync filter rules treat *, ?, and [ as wildcards, so removing a[1].txt
    # could remove a1.txt instead. Until escaping lands, such files are not
    # offered for deletion.
    Given that the file "a[1].txt" has been added on the remote
    When  I run "csync ./project user@host:/project"
    Then  no actions should be reported
    And   the reported change count should be 0
    And   the file "a[1].txt" should still exist on the remote

  @remote @wip
  Scenario: A selected filename containing a newline transfers intact
    # A newline in a filename must not split it into two entries. See SECURITY.md.
    Given that a file whose name contains a newline has been added locally
    When  I run "csync ./project user@host:/project" and respond with "a"
    Then  the reported sync count should be 1
    And   that file should be identical between local and remote
