Feature: Prune run logs

  In order to keep the run logs useful rather than merely numerous — so that when
  I go looking for what a sync did last Tuesday I am reading a short list and not
  excavating one — I want csync to keep only its most recent logs and discard the
  rest as it goes.

  Background:
    Given a local directory containing these files:
      """
      src/main.go
      README.md
      """

  Scenario: A run below the limit prunes nothing
    Given 5 run logs already exist
    When  I run "csync ./project user@host:/project"
    Then  the log directory should hold 6 run logs

  Scenario: A run at the limit drops the oldest log
    # csync keeps the newest 25 logs. This run's own log is always the newest, so
    # it is never the one removed.
    Given 25 run logs already exist
    When  I run "csync ./project user@host:/project"
    Then  the log directory should hold 25 run logs

  Scenario: The run logs that survive pruning are the newest
    # Age comes from the timestamp in each filename, not the modification time,
    # which any backup or copy of the directory rewrites.
    Given 30 run logs already exist
    When  I run "csync ./project user@host:/project"
    Then  the surviving run logs should be the newest ones

  Scenario: Pruning leaves files that are not run logs alone
    # The log directory belongs to the user and deletion can't be undone, so
    # pruning touches only the names csync itself writes.
    Given 30 run logs already exist
    And   a file that is not a run log in the log directory
    When  I run "csync ./project user@host:/project"
    Then  that file should still be there

  Scenario: A run records which logs it pruned
    # A log that vanished without explanation looks like one never written.
    # Naming the pruned files tells the two apart.
    Given 30 run logs already exist
    When  I run "csync ./project user@host:/project"
    Then  the log should name the run logs it pruned

  Scenario: A run that pruned nothing records that it pruned nothing
    # Recording it every time tells a run that pruned nothing apart from one whose
    # pruning never ran.
    Given 5 run logs already exist
    When  I run "csync ./project user@host:/project"
    Then  the log should record that nothing was pruned
