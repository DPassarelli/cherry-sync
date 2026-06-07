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
    #   - TODO.md exercises uppercase-letter placement in the letter group. The
    #     case-only TODO.md/todo.md tie is deliberately NOT tested here: the two
    #     names collapse to a single file on a case-insensitive filesystem (macOS
    #     APFS), so the fixture can't represent both cross-platform. That rule is
    #     pinned filesystem-free by TestComparePaths_UpperBeforeLowerOnCaseTie in
    #     internal/compare.
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
      | create | src/adder.go          |
      | create | src/parser.go         |

  Scenario: Each reported change is labeled with its selection number
    # The number a user types at the prompt to pick a change is shown next to
    # that change, counting from 1 in the displayed (tree) order — so "1" always
    # refers to the first row. This makes the select-by-number affordance in
    # select-and-sync.feature usable: today the selection logic indexes the
    # sorted list correctly, but the list is printed without visible numbers.
    #
    # Coupling note for when this is drilled in: the rendered index becomes part
    # of what the output-parser facade reads. A new step (e.g. "should be
    # numbered, in order") plus an index field on the parsed action keep the
    # existing verb/path assertions in compare-directories and the scenario
    # above from breaking on the changed line shape.
    Given a local directory containing these files:
      """
      README.md
      src/adder.go
      """
    And   an empty remote directory
    When  I run "csync ./project user@host:/project"
    Then  the reported changes should be numbered, in order:
      | number | action | path         |
      | 1      | create | README.md    |
      | 2      | create | src/adder.go |

  # ---------------------------------------------------------------------------
  # TODO: Additional ordering scenarios, not yet drafted.
  # ---------------------------------------------------------------------------
  #
  # - Ordering is independent of verb: a fixture mixing creates, updates, and
  #   deletes is still sorted purely by path, not grouped by action.
