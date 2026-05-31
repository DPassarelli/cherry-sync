Feature: Order the reported actions

  In order to scan the differences predictably and refer to them by number
  when selecting which to sync, I want the reported actions in a stable,
  human-friendly order — not rsync's raw emit order, which groups each
  directory's contents together so a nested file can appear far from where a
  reader scanning a flat list would expect it.

  # Contract: actions are ordered the way a file tree presents them, applying
  # these keys in order at each path level:
  #   1. dot entries (name begins with '.') before non-dot entries
  #   2. within each, files before subdirectories
  #   3. number-leading names before letter-leading names
  #   4. numbers compared by value (2 < 10), letters alphabetically
  #      (case-insensitive, byte order breaking case-only ties)
  # This decouples both the displayed list and the selection numbering from
  # rsync's --itemize emit order, which groups directory contents differently.

  Scenario: Reported actions are ordered like a file tree
    # The fixture mixes the path shapes that exercise the rule:
    #   - dot group leads; within it the dot file (.gitignore) precedes the dot
    #     directory (.config/) — files before directories
    #   - numeric names sort by value, not lexically: 01 < 2 < 10
    #   - number-leading names precede letter-leading names
    #   - mixed case interleaves among letters: main.go between LICENSE and README.md
    #   - TODO.md / todo.md tie under case-fold; byte order puts 'T' before 't'
    #   - nested src/* sorts after every top-level file (files before subdirs)
    # An empty remote makes every file a clean "create", isolating the ordering
    # of paths from any verb differences.
    Given a local directory containing these files:
      """
      .gitignore
      .config/settings.toml
      01-setup.md
      10-data.csv
      2-config.yml
      LICENSE
      main.go
      README.md
      TODO.md
      todo.md
      src/adder.go
      src/parser.go
      """
    And   an empty remote directory
    When  I run "csync ./project user@host:/project"
    Then  the reported actions should be, in order:
      | action | path                  |
      | create | .gitignore            |
      | create | .config/settings.toml |
      | create | 01-setup.md           |
      | create | 2-config.yml          |
      | create | 10-data.csv           |
      | create | LICENSE               |
      | create | main.go               |
      | create | README.md             |
      | create | TODO.md               |
      | create | todo.md               |
      | create | src/adder.go          |
      | create | src/parser.go         |

  # ---------------------------------------------------------------------------
  # TODO: Additional ordering scenarios, not yet drafted.
  # ---------------------------------------------------------------------------
  #
  # - Ordering is independent of verb: a fixture mixing creates, updates, and
  #   deletes is still sorted purely by path, not grouped by action.
