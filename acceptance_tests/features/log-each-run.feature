Feature: Log each run

  In order to troubleshoot a sync after the fact — including one that removed
  files and so cannot be re-run to reproduce it — I want csync to record what it
  did to a log file on this machine, without my having to ask for it first.

  Background:
    Given a local directory containing these files:
      """
      src/main.go
      README.md
      """

  Scenario: A run that compares writes a run log and says where
    When  I run "csync ./project user@host:/project"
    Then  csync should report where it logged the run
    And   a run log should exist at the reported path

  Scenario: The log is written as csync runs, not when it ends
    # csync can be killed at any moment, and the runs worth reading are the ones
    # that ended badly. A log written only on exit would be empty exactly then.
    Given that a file has been changed locally
    And   I have started csync but not yet answered the prompt
    When  I look for the log file
    Then  the log file should already have content

  Scenario: csync reports the log it has been writing all along
    Given that a file has been changed locally
    And   I have started csync but not yet answered the prompt
    And   I have taken note of where the log file is
    When  I answer the prompt
    Then  the reported log path should be the one I found earlier

  Scenario: By the time it prompts, the log names the version that ran
    # Which build ran is what a bug report most often omits.
    Given that a file has been changed locally
    And   I have started csync but not yet answered the prompt
    When  I look for the log file
    Then  the log should record that the version was "0.0.0-test"

  Scenario: The log records the comparison csync ran
    # A run that deleted files can't be repeated, so the logged dry run is the only
    # record of how the change list came about.
    Given that a file has been changed locally
    And   I have started csync but not yet answered the prompt
    When  I look for the log file
    Then  the log should record running "rsync" for the comparison

  Scenario: A completed sync records the transfer that moved the files
    Given that a file has been changed locally
    And   I have started csync but not yet answered the prompt
    When  I answer the prompt
    Then  the log should record the transfer that ran

  Scenario: A completed sync records the removal that pruned the destination
    Given that the file "README.md" has been deleted locally
    And   I have started csync but not yet answered the prompt
    When  I answer the prompt
    Then  the log should record the removal that ran

  @git
  Scenario: In a git work tree, the log records the query for ignore rules
    # Shows someone puzzling over a file that never synced that git's ignore rules
    # were consulted.
    Given a local git repository containing these files:
      """
      src/main.go
      README.md
      """
    When  I run "csync ./project user@host:/project"
    Then  the log should record running "git" for the ignore rules

  Scenario: A path containing a space is logged as a single argument
    Given a local directory whose path contains a space
    When  I run "csync ./project user@host:/project"
    Then  the log should record that source path as one argument

  Scenario: A path containing a double quote is logged without forging a boundary
    # The log delimits arguments with double quotes, so an unescaped quote inside
    # a path would make the log misreport what csync ran.
    Given a local directory whose path contains a double quote
    When  I run "csync ./project user@host:/project"
    Then  the log should record that source path as one argument

  Scenario: A comparison that fails records the exit code it failed with
    Given a local source path that does not exist
    And   an empty remote directory
    When  I run "csync ./project user@host:/project"
    Then  the log should record the comparison's failing exit code

  Scenario: A comparison that fails records what rsync said
    Given a local source path that does not exist
    And   an empty remote directory
    When  I run "csync ./project user@host:/project"
    Then  the log should record what rsync said about the failure

  Scenario: A run that fails at the comparison still says where it logged
    # With no report to print, the path goes to stderr beside the error.
    Given a local source path that does not exist
    And   an empty remote directory
    When  I run "csync ./project user@host:/project"
    Then  csync should report where it logged the failed run

  Scenario: A run that fails during the transfer still says where it logged
    Given that a file has been changed locally
    And   I have started csync but not yet answered the prompt
    And   the changed file is deleted before I answer
    When  I answer the prompt
    Then  csync should report where it logged the failed run

  Scenario: A command's duration is logged as whole milliseconds
    # Rounded up, so a sub-millisecond command never reads as taking no time.
    When  I run "csync ./project user@host:/project"
    Then  the logged duration should be a positive whole number of milliseconds

  Scenario: By the time it prompts, the log lists the changes csync classified
    Given that a file has been changed locally
    And   that the file "src/main.go" has been deleted locally
    And   I have started csync but not yet answered the prompt
    When  I look for the log file
    Then  the log should record 2 classified changes
    And   the classified changes should include "update" of "README.md"
    And   the classified changes should include "delete" of "src/main.go"

  Scenario: The log records the selection apart from the classification
    Given that a file has been changed locally
    And   that the file "src/main.go" has been deleted locally
    When  I run "csync ./project user@host:/project" and respond with "n"
    Then  the log should record 2 classified changes
    And   the log should record 0 selected changes

  Scenario: An applied removal is on record in what the user selected
    Given that a file has been changed locally
    And   that the file "src/main.go" has been deleted locally
    And   I have started csync but not yet answered the prompt
    When  I answer the prompt
    Then  the log should record 2 selected changes
    And   the selected changes should include "delete" of "src/main.go"

  @git
  Scenario: The log names the files csync held out of the comparison
    Given a local git repository containing these files:
      """
      src/main.go
      README.md
      """
    And   the repository's ".gitignore" contains:
      """
      *.log
      """
    And   that the file "debug.log" has been added locally
    When  I run "csync ./project user@host:/project"
    Then  the log should record "debug.log" among the excluded paths
    And   the log should record that the .git directory was excluded

  Scenario: The log records the command line as it was invoked
    # On an explicit run this matches the operands. Under `csync push` or
    # `csync pull` it is the only line that still shows the verb.
    When  I run "csync ./project user@host:/project"
    Then  the log should record the command line that was run

  Scenario: The log names both operands of the run
    When  I run "csync ./project user@host:/project"
    Then  the log should name the source and destination csync reported

  @remote
  Scenario: Under a saved-target push, the log keeps the verb and the resolved operands apart
    Given a ".csync.toml" in the project directory containing:
      """
      remote = "user@host:/project"
      """
    When  I run "csync push" from the project directory
    Then  the log should record the command line that was run
    And   the log should name the source and destination csync reported

  Scenario: A log that cannot be written does not stop the sync
    # The log is a diagnostic, never a precondition: a read-only state directory
    # says nothing about whether the files should move.
    Given that csync cannot write its log
    And   that a file has been changed locally
    And   I have started csync but not yet answered the prompt
    When  I answer the prompt
    Then  the changed file should be identical between local and remote

  Scenario: csync warns when it cannot write a log
    Given that csync cannot write its log
    And   that a file has been changed locally
    And   I have started csync but not yet answered the prompt
    When  I answer the prompt
    Then  csync should warn that it could not write a run log

  Scenario: A run that could not log says so again when it ends
    # A long change list can scroll the first warning away.
    Given that csync cannot write its log
    And   that a file has been changed locally
    And   I have started csync but not yet answered the prompt
    When  I answer the prompt
    Then  csync should say last of all that the run was not logged

  Scenario: csync names no log when it wrote none
    Given that csync cannot write its log
    And   that a file has been changed locally
    And   I have started csync but not yet answered the prompt
    When  I answer the prompt
    Then  csync should not report where it logged the run

  Scenario: --version writes no log
    # Shell completions and package managers probe the binary this way, and that
    # should leave nothing on disk.
    When I run "csync --version"
    Then no run log should have been written

  Scenario: --license writes no log
    When I run "csync --license"
    Then no run log should have been written

  Scenario: A usage error writes no log
    When I run "csync"
    Then no run log should have been written for the rejected run

  Scenario: A run rejected before it reaches rsync writes no log
    When I run "csync ./project host:~alice/x"
    Then no run log should have been written for the rejected run

  Scenario: The log is written under the XDG state directory
    Given the environment variable XDG_STATE_HOME is set
    When  I run "csync ./project user@host:/project"
    Then  the run log should be under "cherry-sync" in $XDG_STATE_HOME

  Scenario: With no XDG_STATE_HOME, the log falls back to the home state directory
    Given the environment variable XDG_STATE_HOME is not set
    When  I run "csync ./project user@host:/project"
    Then  the run log should be under "cherry-sync" in ~/.local/state

  Scenario: The log is kept private to the user
    # The log names every path a run touched, which reveals the shape of the
    # user's work tree.
    When  I run "csync ./project user@host:/project"
    Then  the run log directory should be accessible only by its owner
    And   the run log file should be accessible only by its owner
