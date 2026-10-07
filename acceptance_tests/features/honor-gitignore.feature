# @git scenarios need `git` on the test host (the local operand is set up as a
# real work tree). The runner skips them when git is absent.
#
# Each not-yet-implemented scenario carries @wip (excluded via the "~@wip" tag
# filter the runner applies); drop a scenario's tag when we drill in.
@git
Feature: Honor .gitignore when comparing

  In order to keep build artifacts and other ignored files out of a sync,
  I want files the local git repository ignores left out of the comparison,
  so the diff shows only files I'd actually consider moving.

  # The local side governs in both directions: csync runs `git ls-files` against
  # the local operand and uses that ignore set for push and pull alike. Each
  # scenario sets up its own local side — most as a git work tree, the no-op case
  # as a plain directory — because the setups diverge enough that a shared
  # Background would mask the very thing some scenarios test (e.g. a Background
  # .gitignore would hide whether .git/info/exclude is honored).

  Scenario: An ignored file is left out of the comparison
    # Teeth: README.md (tracked, changed) MUST report; debug.log (ignored, newly
    # added) MUST NOT. Remove the exclusion and debug.log surfaces as a `create`
    # and the count becomes 2 — red. Break compare entirely and the README.md row
    # vanishes — also red. So this fails for the right reason and proves the
    # exclusion is *targeted*, not a blanket "report nothing".
    Given a local git repository containing these files:
      """
      src/main.go
      README.md
      """
    And   the repository's ".gitignore" contains:
      """
      *.log
      """
    And   that the file "README.md" has been changed locally
    And   that the file "debug.log" has been added locally
    When  I run "csync ./project user@host:/project"
    Then  the reported actions should be:
      | action | path      |
      | update | README.md |
    And   the reported change count should be 1

  Scenario: The comparison names the ignored change it withheld
    # Teeth: hiding debug.log is silent unless csync says so, and disclosure is
    # the user's only signal since there's no opt-out flag. The row names debug.log
    # and the action csync declined, even though the action list (README.md only)
    # is identical to the scenario above. Drop the disclosure and this goes red
    # while the actions stay green, proving it is reported in its own right.
    Given a local git repository containing these files:
      """
      src/main.go
      README.md
      """
    And   the repository's ".gitignore" contains:
      """
      *.log
      """
    And   that the file "README.md" has been changed locally
    And   that the file "debug.log" has been added locally
    When  I run "csync ./project user@host:/project"
    Then  the withheld changes should be:
      | action | path      |
      | create | debug.log |

  Scenario: A gitignored .csync.toml is disclosed once, not counted twice
    # csync withholds its own .csync.toml unconditionally, and a project with a saved
    # target commonly gitignores it too — so git reports it as ignored as well. It is
    # still one exclusion: counted among the gitignored paths on top of its own
    # disclosure, it would be announced twice, and a user reading ".csync.toml" plus
    # "2 gitignored paths" would think three files were held back when only two were.
    #
    # Teeth: the withheld set is debug.log alone, and the run log's excluded list
    # holds that one path. Let .csync.toml through into the gitignored set and the
    # log lists it a second time, while its own disclosure stays green — exactly the
    # double-count this pins. The log carries the teeth because .csync.toml is
    # excluded unconditionally and so can never surface as a withheld change.
    Given a local git repository containing these files:
      """
      src/main.go
      README.md
      """
    And   the repository's ".gitignore" contains:
      """
      *.log
      .csync.toml
      """
    And   a ".csync.toml" in the project directory containing:
      """
      remote = "user@host:/project"
      """
    And   that the file "debug.log" has been added locally
    When  I run "csync ./project user@host:/project"
    Then  the log should record the excluded paths:
      | debug.log |
    And   the .csync.toml file should be reported as excluded

  Scenario: An ignored file that changed is reported as withheld
    # Teeth for #59: an ignored file that WOULD have moved is the only exclusion a
    # user gets surprised by, and today it vanishes into a count. The row names the
    # change csync declined to make. Restore the file-level pre-filter and rsync
    # never compares .env, so no withheld row can be produced — red.
    Given a local git repository containing these files:
      """
      src/main.go
      .env
      """
    And   the repository's ".gitignore" contains:
      """
      .env
      """
    And   that the file ".env" has been changed locally
    When  I run "csync ./project user@host:/project"
    Then  the withheld changes should be:
      | action | path |
      | update | .env |

  Scenario: An ignored file that matches on both sides is not reported as withheld
    # The block reports withheld CHANGES, not withheld paths: an ignored file that
    # is already identical was never going to move, so naming it is noise. Report
    # every ignored path instead of only the changed ones and .env appears here — red.
    Given a local git repository containing these files:
      """
      src/main.go
      .env
      """
    And   the repository's ".gitignore" contains:
      """
      .env
      """
    And   that the file "src/main.go" has been changed locally
    When  I run "csync ./project user@host:/project"
    Then  no withheld changes should be reported

  Scenario: A change inside an ignored directory is not reported as withheld
    # Ignored directories stay pre-excluded so rsync never walks them — a measured
    # 5x on a large one — which is the deliberate limit of this disclosure: nobody is
    # surprised that build/ did not sync. Drop the directory pre-filter to surface
    # these and the walk cost comes back with them.
    Given a local git repository containing these files:
      """
      src/main.go
      build/output.bin
      """
    And   the repository's ".gitignore" contains:
      """
      build/
      """
    And   that the file "build/output.bin" has been changed locally
    When  I run "csync ./project user@host:/project"
    Then  no withheld changes should be reported

  Scenario: A non-repository local side excludes nothing
    # Teeth: this local directory is NOT a git work tree, yet it carries a
    # .gitignore naming *.log. Because the trigger is "is a git work tree?" — not
    # "does a .gitignore exist?" — nothing is excluded: debug.log surfaces as a
    # create alongside the README.md update, and no disclosure line is printed.
    # Drop the work-tree guard and `git ls-files` runs against a non-repo, which
    # errors — the comparison fails outright and reports no actions at all (red).
    # The guard is what keeps a non-repo a clean no-op. Runs only where git is
    # installed (@git), so a pass proves the gate is work-tree membership, not the
    # git binary simply being absent.
    Given a local directory containing these files:
      """
      src/main.go
      README.md
      """
    And   the directory's ".gitignore" contains:
      """
      *.log
      """
    And   that the file "README.md" has been changed locally
    And   that the file "debug.log" has been added locally
    When  I run "csync ./project user@host:/project"
    Then  the reported actions should be:
      | action | path      |
      | update | README.md |
      | create | debug.log |
    And   the reported change count should be 2
    And   no withheld changes should be reported
    And   the .git directory should not be reported as excluded

  Scenario: A file ignored via .git/info/exclude is left out
    # Teeth: this repository has NO .gitignore at all — the ignore rule lives only
    # in .git/info/exclude (git's per-clone, uncommitted ignore list). debug.log
    # must still be excluded, because csync drives `git ls-files --exclude-standard`,
    # which honors .gitignore, .git/info/exclude, and global excludes alike. Key the
    # exclusion on the literal presence of a .gitignore file and this repo has none,
    # so debug.log would surface and the count become 2 — red. Proves the trigger is
    # "is a git work tree?" + --exclude-standard, not "is there a .gitignore?".
    Given a local git repository containing these files:
      """
      src/main.go
      README.md
      """
    And   the repository's ".git/info/exclude" contains:
      """
      *.log
      """
    And   that the file "README.md" has been changed locally
    And   that the file "debug.log" has been added locally
    When  I run "csync ./project user@host:/project"
    Then  the reported actions should be:
      | action | path      |
      | update | README.md |
    And   the reported change count should be 1

  Scenario: A top-level ignore does not float onto a same-named nested path
    # Teeth — the leading-"/" anchoring in gitignoreExcludes. The repo ignores the
    # top-level build/ (via "/build/", anchored to the repo root by git), but a
    # *different* src/build/ is NOT ignored. Both hold a changed file. Anchoring
    # each emitted exclude with a leading "/" pins it to the transfer root, so the
    # exclude hits the real top-level build/artifact.o (absent) yet leaves
    # src/build/keep.go (present). Drop the "/" prefix and rsync reads "build/" as
    # a floating basename match at any depth — it swallows src/build/ too and
    # keep.go vanishes (red). Break exclusion entirely and build/artifact.o
    # reappears as a second change (also red). Verified against GNU rsync 3.4.1 and
    # openrsync v29: both float an unanchored "build/" and both honor "/build/".
    Given a local git repository containing these files:
      """
      src/main.go
      README.md
      src/build/keep.go
      build/artifact.o
      """
    And   the repository's ".gitignore" contains:
      """
      /build/
      """
    And   that the file "build/artifact.o" has been changed locally
    And   that the file "src/build/keep.go" has been changed locally
    When  I run "csync ./project user@host:/project"
    Then  the reported actions should be:
      | action | path              |
      | update | src/build/keep.go |
    And   the reported change count should be 1

  Scenario: The local .git directory is never offered for sync
    # Teeth — the explicit "/.git/" exclude. git never reports its own .git/ as
    # ignored (it special-cases that directory), so without an explicit exclude a
    # push from a repo offers every .git/ object for transfer — pure noise, and it
    # would clobber the other side's git state (HEAD, index, refs, hooks). Pushing
    # a fresh repo (real .git/ from git init) to an empty remote, only the working
    # files are offered; the whole .git/ tree is gone. Drop the "/.git/" pattern and
    # dozens of .git/ creates flood the list (red). Verified on GNU rsync 3.4.1 that
    # an anchored "/.git/" exclude zeroes them out.
    Given a local git repository containing these files:
      """
      src/main.go
      README.md
      """
    And   an empty remote directory
    When  I run "csync ./project user@host:/project"
    Then  the reported actions should be:
      | action | path        |
      | create | README.md   |
      | create | src/main.go |
    And   the reported change count should be 2

  Scenario: The .git exclusion is disclosed even when nothing is gitignored
    # The .git/ exclusion is invisible by mechanism — git never lists it — so csync
    # must announce it, and must do so even when there's no .gitignore at all (zero
    # gitignored paths). This repo has no .gitignore; the disclosure still reports
    # the .git directory, with no gitignored-path count. Drop the disclosure, or
    # gate it on a gitignored count > 0, and this goes red.
    Given a local git repository containing these files:
      """
      src/main.go
      README.md
      """
    And   an empty remote directory
    When  I run "csync ./project user@host:/project"
    Then  the .git directory should be reported as excluded
    And   no withheld changes should be reported

  Scenario: A submodule's nested .git metadata is never offered for sync
    # Teeth — the FLOATING ".git" exclude (no leading slash), covering the
    # submodule case the anchored "/.git/" misses. A checked-out submodule carries
    # its own .git *file* (a "gitdir:" pointer, not a directory) deep in the tree —
    # here vendor/lib/.git. Pushing a fresh superproject to an empty remote, only
    # the working files are offered; the top-level .git/ AND the submodule's nested
    # .git are gone. Keep only the anchored "/.git/" and vendor/lib/.git floods back
    # as a third create, count 3 (red). "Simplify" the floating exclude to a
    # dir-only ".git/" and the submodule's .git *file* leaks the same way — a
    # trailing slash matches directories only (red). Verified on GNU rsync 3.4.1
    # that a floating ".git" (no slashes) drops both a nested .git dir and file.
    Given a local git repository containing these files:
      """
      README.md
      vendor/lib/lib.go
      vendor/lib/.git
      """
    And   an empty remote directory
    When  I run "csync ./project user@host:/project"
    Then  the reported actions should be:
      | action | path              |
      | create | README.md         |
      | create | vendor/lib/lib.go |
    And   the reported change count should be 2

  @remote
  Scenario: Pull direction — the local repo's ignore set still governs
    # "Local repo governs both directions": on a pull (remote -> local) csync still
    # derives the ignore set from the LOCAL side — here the destination — because
    # localSyncDir picks the non-remote operand. The remote-only notes.txt is
    # pulled (create); debug.log, which differs and would otherwise be received, is
    # held back because the local repo ignores *.log. @remote keeps the source's
    # host: spec so csync sees it as remote and localSyncDir resolves to the local
    # destination. Teeth: make localSyncDir always pick the source and git runs
    # against the remote spec (not a local dir) -> no exclusion -> debug.log pulled
    # and the count becomes 2 (red); breaking exclusion entirely does the same.
    # Verified the pull exclude behavior by experiment (GNU rsync 3.4.1).
    #
    # Scope: covers a file present on BOTH sides (ignored locally). A remote-ONLY
    # ignored file isn't caught by this ls-files pre-filter — the next scenario
    # covers that case via a git check-ignore pass.
    Given a local git repository containing these files:
      """
      src/main.go
      README.md
      debug.log
      """
    And   the repository's ".gitignore" contains:
      """
      *.log
      """
    And   that the file "debug.log" has been changed locally
    And   that the file "notes.txt" has been added on the remote
    When  I run "csync user@host:/project ./project"
    Then  the reported actions should be:
      | action | path      |
      | create | notes.txt |
    And   the reported change count should be 1

  @remote
  Scenario: Pull direction — a remote-only file the local repo ignores is held back
    # Sibling to the pull scenario above, which covers a file present on BOTH sides.
    # Here the ignored file exists ONLY on the remote and not yet locally. `git
    # ls-files` lists only files in the LOCAL tree, so the pre-comparison
    # --exclude pre-filter can't see secret.log and it would be pulled. csync additionally
    # runs the comparison's result paths through `git check-ignore` in the local
    # repo, which evaluates a path against the ignore rules WITHOUT the file needing
    # to exist locally — so the remote-only secret.log is dropped while notes.txt
    # (not ignored) is offered. Teeth: remove the check-ignore post-filter and
    # secret.log returns as a second create, the change count becomes 2, and the
    # withheld row vanishes (secret.log accounted for nowhere) — red on all three.
    # Verified by experiment that `git check-ignore` flags a non-existent path yet
    # honors the index (a force-added tracked *.log would NOT be dropped).
    Given a local git repository containing these files:
      """
      src/main.go
      README.md
      """
    And   the repository's ".gitignore" contains:
      """
      *.log
      """
    And   that the file "secret.log" has been added on the remote
    And   that the file "notes.txt" has been added on the remote
    When  I run "csync user@host:/project ./project"
    Then  the reported actions should be:
      | action | path      |
      | create | notes.txt |
    And   the reported change count should be 1
    And   the withheld changes should be:
      | action | path       |
      | create | secret.log |

  @remote
  Scenario: Pull direction — a folder of ignored files still offers a remote file that isn't ignored
    # A content rule like logs/*.log ignores files, not the folder. But when every
    # file in the local folder happens to match it, `git ls-files --directory`
    # reports the folder alongside its files (verified by experiment), and csync
    # used to turn that into an --exclude for the whole folder. A remote file the
    # rule does not match was then never compared, so it could never be offered,
    # and nothing said why (#118). Teeth: exclude every folder ls-files reports,
    # without asking git whether the folder itself is ignored, and notes.txt is
    # never offered.
    Given a local git repository containing these files:
      """
      src/main.go
      logs/old.log
      """
    And   the repository's ".gitignore" contains:
      """
      logs/*.log
      """
    And   that the file "logs/notes.txt" has been added on the remote
    When  I run "csync user@host:/project ./project"
    Then  the reported actions should be:
      | action | path           |
      | create | logs/notes.txt |

  @remote
  Scenario: Pull direction — a re-included file in a folder of ignored files is still offered
    # The same hole as #118 through a different rule shape: `logs/*` with a re-include
    # for the README, a common way to keep a generated folder documented. Every local
    # file in logs/ is ignored, so `git ls-files --directory` reports the folder, and
    # csync must ask git whether the folder itself is ignored. Asked as "logs/" with a
    # trailing slash, git matches `logs/*` against it (the `*` matching the empty name
    # after the slash) and would wrongly confirm it. Teeth: query the folder with its
    # trailing slash and README.md is never offered.
    Given a local git repository containing these files:
      """
      src/main.go
      logs/old.log
      """
    And   the repository's ".gitignore" contains:
      """
      logs/*
      !logs/README.md
      """
    And   that the file "logs/README.md" has been added on the remote
    When  I run "csync user@host:/project ./project"
    Then  the reported actions should be:
      | action | path           |
      | create | logs/README.md |

  @remote
  Scenario: Many withheld changes under one top-level folder are summarized
    # An ignored directory that exists only on the remote of a pull is invisible to
    # `git ls-files`, so it is never pre-excluded: rsync itemizes every file in it and
    # `git check-ignore` withholds each one. A real .ansible/ ran to thousands of rows
    # and buried the changes on offer. Past ten rows, a top-level folder's withheld
    # changes become one summary row. The grouping is display-only, so it never
    # claims the folder itself is ignored. Teeth: drop the grouping and eleven file
    # rows come back with no summary.
    Given a local git repository containing these files:
      """
      src/main.go
      """
    And   the repository's ".gitignore" contains:
      """
      build/
      """
    And   that 11 files have been added on the remote under "build/a"
    When  I run "csync user@host:/project ./project"
    Then  the withheld changes should be summarized as:
      | folder | count | actions |
      | build/ | 11    | create  |

  @remote
  Scenario: Ten or fewer withheld changes under one folder are listed individually
    # The threshold's other edge. A handful of rows costs little screen, and naming
    # each file is what tells a user which one they were missing (#59). Teeth: lower
    # the threshold to ten or fewer and these rows fold into a summary.
    Given a local git repository containing these files:
      """
      src/main.go
      """
    And   the repository's ".gitignore" contains:
      """
      build/
      """
    And   that 10 files have been added on the remote under "build/a"
    When  I run "csync user@host:/project ./project"
    Then  the withheld changes should list 10 individual files

  @remote
  Scenario: A summary counts each action separately
    # A folder whose contents are ignored but which holds a file that isn't is never
    # pre-excluded, on either side, so a pull can withhold creates and deletes from
    # it together. A single verb would misstate half the rows. The README keeps `git
    # ls-files` from reporting logs/ as a wholly ignored directory, which would hide
    # the folder from the comparison altogether. Teeth: report only the
    # first row's verb and the summary reads "create" alone.
    Given a local git repository containing these files:
      """
      src/main.go
      logs/README.md
      """
    And   the repository's ".gitignore" contains:
      """
      logs/*.log
      """
    And   that 6 files have been added on the remote under "logs"
    And   that 5 files have been added locally under "logs"
    When  I run "csync user@host:/project ./project"
    Then  the withheld changes should be summarized as:
      | folder | count | actions             |
      | logs/  | 11    | 6 create · 5 delete |

  @remote
  Scenario: The run log records every withheld file, even when summarized
    # The summary is a screen-saving measure only. The run log is where a user goes
    # to find out whether one particular file was held back, so it keeps every path.
    # This guards the design rather than code that exists today: grouping lives in
    # the view, so it passes from the start. Teeth: move the grouping upstream of
    # the run log and the log records one entry instead of eleven.
    Given a local git repository containing these files:
      """
      src/main.go
      """
    And   the repository's ".gitignore" contains:
      """
      build/
      """
    And   that 11 files have been added on the remote under "build/a"
    When  I run "csync user@host:/project ./project"
    Then  the log should record 11 excluded paths

  @remote
  Scenario: Pull direction — a remote repository's .git is never offered for sync
    # The mirror of "The local .git directory is never offered for sync", and the
    # case #103 reports. The .git exclusion is gated on the LOCAL side being a work
    # tree, but on a pull the local side is the destination — so pulling from a repo
    # into a plain directory passes no exclude at all and rsync offers the remote's
    # entire .git/ tree (config, HEAD, the sample hooks, every object) as creates.
    # The exclusion belongs to whichever side is being read, not to whichever side
    # happens to be local. Teeth: gate the ".git" exclude on isGitWorkTree again and
    # dozens of .git/ creates flood the list (red). Verified by experiment (GNU
    # rsync 3.4.1, and openrsync on macOS) that a floating ".git" --exclude drops
    # the sender's metadata whichever end of the connection the sender is on.
    Given a local directory containing these files:
      """
      README.md
      """
    And   a remote git repository containing these files:
      """
      README.md
      src/main.go
      """
    When  I run "csync user@host:/project ./project"
    Then  the reported actions should be:
      | action | path        |
      | create | src/main.go |
    And   the reported change count should be 1

  @remote
  Scenario: Pull direction — the remote repository's .git exclusion is disclosed
    # Sibling to the scenario above, which asserts the .git/ tree is gone from the
    # change list; this one asserts csync says so. The disclosure must follow the
    # evidence rather than the work-tree check: the local side here is a plain
    # directory, so a disclosure gated on isGitWorkTree stays silent and withholds
    # the remote's .git/ without a word — the one thing csync promises never to do.
    # No .gitignore exists on either side, so the .git directory must be the only
    # thing reported. Teeth: report GitDirExcluded from the local work-tree check
    # and this goes red while the scenario above still passes.
    Given a local directory containing these files:
      """
      README.md
      """
    And   a remote git repository containing these files:
      """
      README.md
      src/main.go
      """
    When  I run "csync user@host:/project ./project"
    Then  the .git directory should be reported as excluded
    And   no withheld changes should be reported

  # ---------------------------------------------------------------------------
  # TODO: sibling scenarios, each its own behavior — drafted as we drill in.
  # ---------------------------------------------------------------------------
  #
  # - Syncing a *subdirectory* of the repo (csync ./repo/sub host:/dst): the
  #   emitted ignore paths must resolve relative to the sync root, not the repo
  #   root — `git ls-files` is run in the sync dir for exactly this reason. The
  #   anchoring half is covered above; this is the run-git-in-the-sync-dir half,
  #   still untested. Needs harness support for a sync operand below the repo root
  #   (today the runCsync placeholder maps only the bare "./project" token).
