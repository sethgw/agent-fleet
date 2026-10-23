# adw-tui: the terminal board

`adw-tui` is a keyboard-driven terminal view of the fleet, built for review rather than monitoring. Run it in
its own herdr tab. It shows what needs you, then each epic's in-flight segments with their diff, every
reviewer's verdict and findings, and PR state, then every agent tab with its live screen.

It is a Go program (`tui/`) and sits beside the web board rather than replacing it. Keep `adw-board --serve`
running if you want the phone view.

## Install

`./install.sh` builds it into `$ADW_BIN_DIR` (default `~/.local/bin`) when Go 1.24+ is on `PATH`, and skips it
otherwise. By hand:

```bash
cd tui && go build -o ~/.local/bin/adw-tui .
```

## Layout

| Section | Items | Detail |
|---|---|---|
| NEEDS YOU | open `adw-ask` decisions, tabs sitting in a dialog (snoozed ones hidden) | the decision's options and body; a dialog's live screen |
| REVIEW · `<epic>` | segments that are ready, building, pr-open, blocked or pr-ready, most urgent first | four tabs: overview, reviews, diff, brief |
| TABS | every herdr agent tab | role, watch state, summary, live screen |
| FIRST MATE | the first mate's digest | the digest |

Each segment row carries badges once its data is loaded: each reviewer's latest verdict (`cl✓`, `cx✗`; a `~`
means that verdict is for an older head than the worktree's) and CI (`CI✓`, `CI…`, `CI✗`).

**overview**: the segment's note and deps; its worktree (branch, head, commits ahead, uncommitted files);
PR state from `pr-state` (CI, review bot, review decision, merge state, open threads, and whether the
worktree has commits that are not pushed); each reviewer's latest verdict; tabs working in the worktree; the
top of the brief.

**reviews**: the findings in each reviewer's latest verdict file, then the whole ledger history for the
branch. A verdict recorded without a report file still shows, and verdicts `review-receipt` marked
`unverified` say so. Focus the detail pane (`tab`) to move between findings; `enter` opens the finding's
file at its line in `$VISUAL`/`$EDITOR`, and `r` sends the epic lead a note that cites it.

**diff**: the worktree, committed and uncommitted, against its merge base with `origin/<base_branch>` from
`manifest.json`. With no worktree, it falls back to `gh pr diff`. `d` opens the full diff in a pager, through
`delta` when it is installed.

## Keys

`?` inside the program lists them all.

| Key | Does |
|---|---|
| `j`/`k`, `g`/`G` | move in the list, or scroll the detail when it has focus |
| `tab` / `esc` | focus the detail pane / go back to the list |
| `n` | next decision or dialog |
| `1`-`4`, `[`/`]` | segment tabs |
| `enter` | decision: answer it. Tab or dialog: jump to it in herdr. Segment: jump to its builder tab, or to the lead |
| `r` | message the tab (for a segment, the epic lead), with a reference to the segment and finding |
| `c` | message the tab working in the segment's worktree |
| `d`, `e`, `o`, `v` | full diff in a pager; edit the brief or open the finding; open the PR in a browser; page the text |
| `z` / `Z` | snooze a decision or dialog for 1h / until 8am (the same `board-snoozes.json` the web board uses) |
| `R` | reload everything, including PR state |

Answering a decision is two steps: pick an option (the recommendation is preselected), then add an
optional note. The answer goes through `adw-ask answer`, so locking and delivery to the asking tab are
exactly the web board's. Messages to tabs go through herdr's `agent.prompt`, and are refused while the tab is
in a dialog.

## Where the data comes from

- **herdr socket** (`$HERDR_SOCKET_PATH`, default `~/.config/herdr/herdr.sock`): an `events.subscribe`
  stream triggers a reload when an agent appears, leaves or changes status. Pane screens (`pane.read`),
  focus and prompts use the socket too. Remote machines use `~/.config/herdr/remote/<name>.sock`, as in
  `docs/multi-machine.md`.
- **`adw-board --json`**, every 3 seconds and on herdr events: tab classification, epic linking, watch state,
  decisions and snoozes. The rules stay in one place.
- **`$ADW_HOME`** directly: `epics/*/graph.json` and briefs, `manifest.json`, `review-ledger.jsonl` and the
  verdict files it points to.
- **`pr-state --json`**, once per segment until you press `R`, run from the repo's checkout.

`ADW_TUI_LOG=/path/to/file` logs every message the program handles, for debugging.
