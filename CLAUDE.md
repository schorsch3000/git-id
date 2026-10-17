# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

# Missing tools
this is a development environment, if any tools are missing ask the human, you will most likely get them!

No known missing tools (git and go installed).

# Project state
Early development. Repo holds the design (`idea.md`), `Dockerfile.az` (the dev environment image, debian:13; nodejs is installed on purpose so Claude Code works, leave it) and JetBrains/GoLand config in `.idea/`. Done: scaffold, store, datapoint/identity CLI, `use`/`current`/`sync`/`ssh`/`init`, e2e push test, completion (bash/zsh/fish), shell hook, TUI, `cleanup`. All planned features are built.

# Decisions
- Language: Go (module `git-id`). CLI with `cobra`, TUI with `bubbletea`/`bubbles`/`lipgloss`.
- Address book: single TOML file at `$XDG_CONFIG_HOME/git-id/book.toml`.
- Identities store labels referencing datapoints, never copies. Deleting a datapoint still in use is refused unless `--force`.
- Applying an identity (`git id use`): repo-local `user.name`, `user.email`, `id.identity` (label), and `core.sshCommand = git id ssh ...`, a thin wrapper that resolves the key from the address book at run time, so key changes propagate. Name/email changes propagate via `git id sync`.
- Tests use a throwaway `HOME` / `GIT_CONFIG_GLOBAL`, never the real git config.


# Commands
- Build: `go build -o bin/git-id ./cmd/git-id` (put `bin/` on PATH to run as `git id`)
- Test all: `go test ./...`; single test: `go test ./internal/cli -run TestSyncPropagatesAndPrunes`
- Vet/format: `go vet ./...`, `gofmt -l .`
- Make targets: `make build` (bin/git-id, static, version embedded), `make test`, `make vet`, `make fmt-check`, `make release` (runs fmt-check, vet, test, then cross-builds linux/darwin/windows x amd64/arm64 into `dist/` as `.tar.gz` / `.zip` plus `SHA256SUMS`; needs `zip`), `make clean`, `make install [PREFIX= DESTDIR=]`. `VERSION` defaults to `git describe --tags --always --dirty`, override with `make release VERSION=1.2.3`; it feeds `git-id --version` via `-X git-id/internal/cli.version`.
- Windows: `git id ssh` cannot `syscall.Exec`, so `internal/cli/exec_windows.go` runs ssh as a child process (untested on real Windows); `exec_unix.go` execs.
- If `$HOME` is not writable by the user, Go cannot create its caches: set `GOCACHE`, `GOPATH` (and `GOFLAGS=-modcacherw`) to a writable directory.
- `git id --help` fails here (git looks for a man page); use `git-id --help` or `git id help`.
- CLI tests run with `--book <tmpfile>` or `$GIT_ID_BOOK`, and `isolate(t)` (internal/cli/use_test.go) sets a throwaway HOME/`GIT_CONFIG_GLOBAL`.

# Layout
- `internal/store`: TOML address book (datapoints + identities by label), reference integrity, atomic writes, repo registry (`repos.txt` next to the book, used by `sync`).
- `internal/gitcfg`: thin wrapper over the `git` binary for config.
- `internal/cli`: cobra commands. `use` applies an identity; hidden `ssh` command is the `core.sshCommand` wrapper that execs `ssh -i <key> -o IdentitiesOnly=yes`.
- `e2e`: real git + real ssh against an in-process ssh server (x/crypto/ssh); builds git-id, shims ssh to skip host-key checks. Needs `git`, `ssh`, `go` on PATH, else skips. Run alone: `go test ./e2e/`. Env gotcha: `GIT_SSH_COMMAND` overrides `core.sshCommand`, so the e2e env strips `GIT_*` and `SSH_*`.
- Completion: `internal/cli/complete.go`. Dynamic label completion is attached centrally in `attachCompletions` (looks commands up by path, panics on rename). Root command is named `git-id` so generated scripts target the binary, not `git`; `git-id completion bash|zsh|fish` each append glue so `git id <TAB>` works too: bash `_git_id`, zsh a `_git-id` wrapper (git's `_git` calls it with words[1]=id), fish a `complete -c git` rule (fish's own custom-command delegation does not fire for git-id). `e2e/completion_test.go` tests all three in real shells (zsh and fish via a zsh `zpty` pseudo-terminal pressing TAB); bash needs the `bash-completion` package; each skips if its shell is absent. Limit: fish glue ignores global git options (`git -C dir id`).
- Hook: `internal/cli/hook.go` prints bash/zsh/fish snippets (`git-id hook <shell>`). They fire on directory change only when the dir directly contains `.git`, call `git-id current -q` and nag on exit status 1 only (127 = binary missing stays silent). Bash hook is tested for real in `e2e/hook_test.go`; zsh (chpwd) and fish (--on-variable PWD) are tested the same way with real shells, skipping if absent.
- TUI: `internal/tui` (bubbletea v1, model in `tui.go`, add/edit dialog in `form.go`), started by `git-id tui` (refuses without a TTY). Tabs for identities/names/emails/keys; enter on an identity applies it to the current repo via `internal/repo.Use`, a/e/d add/edit/delete with the same integrity rules as the CLI (deleting a used datapoint asks, then removes dependents). Every change reloads the book from disk, applies, saves (no clobbering of concurrent CLI edits). Tests: model-level in `internal/tui/tui_test.go` (feed `tea.KeyMsg`, assert book/git config/view); real pty session in `e2e/tui_test.go` (zsh `zpty`; bubbletea reads a multi-key write as one paste, so the driver sends keys one by one).
- `internal/repo`: shared by CLI and TUI: `Apply`, `Identity`, `Use`; `store.ResolveKeyPath` validates key files.
- Cleanup: `git id cleanup` (`internal/cleanup` + `internal/cli/cleanup.go`; spec and decisions at the end of idea.md). `Scan`/`Find` compare history against the address book, `Ask` is the interactive y/n/all/skip/quit loop, `Rewrite` shells out to `git filter-repo --partial --force` with name/email callbacks (python bytes literals, every byte hex-escaped). Needs `git-filter-repo` (external, not bundled). Safety: loud all-refs banner, dirty-tree refusal, typed "yes", bundle backup in `.git` + restore command, never pushes. Tests: `internal/cleanup` (units) and `internal/cli/cleanup_test.go` (real repos, real filter-repo, restore command is executed).

# Design intent (from idea.md)
`git-id`: git identity tool. Problem: one user, multiple names, email addresses and SSH keys across different git contexts.

- Binary named `git-id` so git runs it as subcommand `git id`.
- Address book holds datapoints: names, email addresses, SSH keys. An *identity* = one name + one email + one SSH key.
- Identities reference datapoints, not copy them: editing a name/email/key propagates to every identity using it.
- `init` command: global setup. Takes existing global git user name and email, removes them from global config, moves them into the address book.
- Shell hook (built, see Layout): on entering a directory with `.git`, check whether an identity is set; if not, tell the user to set one.
- CLI: intuitive command language, generated shell autocompletion covering all datapoints.
- TUI: choose an identity, manage datapoints and identities.
- Applying an identity: name/email = repo-local git config. SSH key selection undecided; leading idea is setting `GIT_SSH_COMMAND` (alternative worth weighing: `core.sshCommand` in repo config, which needs no env hook).
