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
    And   that all of the files are identical between local and remote

  Scenario: A run below the limit prunes nothing
    # Pruning is a ceiling, not a quota: a directory with room to spare is left
    # exactly as it was found, and the run adds its own log to it. Five existing
    # logs plus this run's is six, which is the whole assertion — a csync that
    # pruned on every run regardless of the count would leave fewer.
    Given 5 run logs already exist
    When  I run "csync ./project user@host:/project"
    Then  the log directory should hold 6 run logs

  Scenario: A run at the limit drops the oldest log
    # The boundary, where an off-by-one lives. Twenty-five already there plus this
    # run's would be twenty-six, one over, so exactly one log has to go. Twenty-five
    # remain: a ceiling applied one too early would leave twenty-four, one applied
    # one too late would leave twenty-six, and both are visible here.
    #
    # This run's own log is never a candidate. It is the newest thing in the
    # directory by the moment pruning happens, which is what makes "keep the newest
    # twenty-five" safe to state without a special case for the log being written.
    Given 25 run logs already exist
    When  I run "csync ./project user@host:/project"
    Then  the log directory should hold 25 run logs

  Scenario: The run logs that survive pruning are the newest
    # Deleting the right number of logs and deleting the right logs are separate
    # things, and only the second is what a reader cares about. A csync that pruned
    # by walk order, or by whatever the filesystem handed back first, would satisfy
    # the count scenarios above while throwing away this morning's run and keeping
    # one from last spring.
    #
    # Order comes from the timestamp in each filename rather than the modification
    # time: the name records when the run actually started, and mtime is rewritten
    # by any backup or copy that touches the directory.
    Given 30 run logs already exist
    When  I run "csync ./project user@host:/project"
    Then  the surviving run logs should be the newest ones

  Scenario: Pruning leaves files that are not run logs alone
    # The log directory belongs to the user, not to csync, and deleting from a
    # directory is the one thing here that cannot be undone. So pruning is confined
    # to names csync itself writes; anything else found there is somebody else's
    # file and survives, however old it is and however full the directory.
    Given 30 run logs already exist
    And   a file that is not a run log in the log directory
    When  I run "csync ./project user@host:/project"
    Then  that file should still be there

  Scenario: A run records which logs it pruned
    # A log that vanished without explanation is indistinguishable from one that was
    # never written, and the second is a bug worth chasing while the first is not.
    # Naming the files it removed lets this run account for their absence, so the
    # surviving logs together explain the whole directory.
    Given 30 run logs already exist
    When  I run "csync ./project user@host:/project"
    Then  the log should name the run logs it pruned

  Scenario: A run that pruned nothing records that it pruned nothing
    # The record is always present, like the excluded-paths line: a missing "pruned"
    # line would be ambiguous between a run that removed nothing and a run whose
    # pruning never happened, and telling those apart is the reason to read the log
    # in the first place.
    Given 5 run logs already exist
    When  I run "csync ./project user@host:/project"
    Then  the log should record that nothing was pruned
