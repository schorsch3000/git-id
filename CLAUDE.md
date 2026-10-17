# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

# Missing tools
this is a development environment, if any tools are missing ask the human, you will most likely get them!

No known missing tools (git and go installed).

# Project state
Early development. Repo holds the design (`idea.md`), `Dockerfile.az` (the dev environment image, debian:13; nodejs is installed on purpose so Claude Code works, leave it) and JetBrains/GoLand config in `.idea/`. All planned features are built: address book, identities, `use`/`unuse`/`current`/`sync`/`rename`/`repos`, ssh wrapper, completion (bash/zsh/fish), shell hook, TUI, `cleanup`, `--json`, man pages, Makefile, CI. Untested: real Windows/macOS runs, a real sshd or forge, the CI workflows on a runner.

# Decisions
- Language: Go (module `git-id`). CLI with `cobra`, TUI with `bubbletea`/`bubbles`/`lipgloss`.
- Address book: single TOML file at `$XDG_CONFIG_HOME/git-id/book.toml`.
- Identities store labels referencing datapoints, never copies. Deleting a datapoint still in use is refused unless `--force`.
- Applying an identity (`git id use`): repo-local `user.name`, `user.email`, `id.identity` (label), and `core.sshCommand = git id ssh ...`, a thin wrapper that resolves the key from the address book at run time, so key changes propagate. When the book is not at the default path the command is `GIT_ID_BOOK='<path>' git id ssh <id>` (git runs it in a shell that knows neither `--book` nor the caller's env); `repo.IsOurSSHCommand` recognises both forms. Name/email changes propagate via `git id sync`.
- Tests use a throwaway `HOME` / `GIT_CONFIG_GLOBAL`, never the real git config.


# Commands
- Build: `go build -o bin/git-id ./cmd/git-id` (put `bin/` on PATH to run as `git id`)
- Test all: `go test ./...`; single test: `go test ./internal/cli -run TestSyncPropagatesAndPrunes`
- Vet/format: `go vet ./...`, `gofmt -l .`
- Make targets: `make build` (bin/git-id, static, version embedded), `make test`, `make vet`, `make fmt-check`, `make man`, `make release` (runs fmt-check, vet, test, then cross-builds linux/darwin/windows x amd64/arm64 into `dist/` as `.tar.gz` / `.zip` plus `SHA256SUMS`; needs `zip`), `make clean`, `make install` / `make uninstall` `[PREFIX= DESTDIR=]` (uninstall removes the binary and `git-id*.1`, leaves directories). `VERSION` defaults to `git describe --tags --always --dirty`, override with `make release VERSION=1.2.3`; it feeds `git-id --version` via `-X git-id/internal/cli.version`.
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
- Help colours: `internal/cli/help.go` replaces cobra's help/usage funcs with a template that styles headings, commands, flags and the examples/warning in long descriptions; the renderer is bound to the output writer, so colours appear only on a terminal (`NO_COLOR` off, `CLICOLOR_FORCE` on, used by tests). `TestHelpWithoutColourMatchesCobraDefault` compares the plain output of every command with cobra's own, so nothing can get lost in the custom template.
- Terminal queries: bubbletea v1 `init()` queries the terminal background on every start (stall on terminals that do not answer, answers leak into the shell). `internal/termquiet` (imported first by `cmd/git-id`) sets the background explicitly to avoid it; `e2e/startup_test.go` runs git-id in a real pty and fails on any query. Do not remove that import.
- TUI: `internal/tui` (bubbletea v1, model in `tui.go`, add/edit dialog in `form.go`), started by `git-id tui` (refuses without a TTY). Tabs for identities/names/emails/keys; enter on an identity applies it to the current repo via `internal/repo.Use`, a/e/d add/edit/delete with the same integrity rules as the CLI (deleting a used datapoint asks, then removes dependents). Every change reloads the book from disk, applies, saves (no clobbering of concurrent CLI edits). Tests: model-level in `internal/tui/tui_test.go` (feed `tea.KeyMsg`, assert book/git config/view); real pty session in `e2e/tui_test.go` (zsh `zpty`; bubbletea reads a multi-key write as one paste, so the driver sends keys one by one). Key picker: in the key add/edit form `ctrl+o` (modePicker, `internal/tui/keys.go`) lists the private keys in `~/.ssh` (`Model.sshDir`, detected by the PEM/OpenSSH header of the first line, not by file name), shows fingerprints and keys already in the book, and fills private path, public path and a suggested label. Every key you can press is drawn with `keySty` (bold bright cyan) and its description faint, in the footer hints (`hints()`/`renderHints`), the `[y/N]` prompt and the empty-list hint; tests force a colour profile (`withColor`) because without a TTY lipgloss emits plain text, and e2e waits must match text inside one styled segment (colour codes sit between a key and its description).
- `internal/repo`: shared by CLI and TUI: `Apply`, `Identity`, `Use`, `Unuse`, `Relabel` (identity rename updates tracked repos), `SSHCommand`, `SSHOverridden` (`$GIT_SSH_COMMAND` beats `core.sshCommand`: `use`, `current`, TUI and the hooks warn; the hook message must escape `$` or the shell expands it); `store.ResolveKeyPath` validates key files.
- Cleanup: `git id cleanup` (`internal/cleanup` + `internal/cli/cleanup.go`; spec and decisions at the end of idea.md). `Scan`/`Find` compare history against the address book, `Ask` is the interactive y/n/all/skip/quit loop, `Rewrite` shells out to `git filter-repo --partial --force` with name/email callbacks (python bytes literals, every byte hex-escaped). Needs `git-filter-repo` (external, not bundled). Safety: loud all-refs banner, dirty-tree refusal, typed "yes", bundle backup in `.git` + restore command, never pushes. Tests: `internal/cleanup` (units) and `internal/cli/cleanup_test.go` (real repos, real filter-repo, restore command is executed).

- Listings/JSON: `internal/cli/output.go` (`--json` on every `ls`/`show`/`repos ls`, empty lists print `[]`; `fingerprint()` reads the stored public key file with x/crypto/ssh). `rename` exists for datapoints (`store.RenameDatapoint`, identities follow) and identities (`repo.Relabel` fixes tracked repos). `unuse` and `repos ls|forget` live in `internal/cli/repos.go`.
- Man pages: `tools/genman` (cobra/doc, kept out of the binary) writes `git-id.1` and one page per command; `make man`/`install`/`release` ship them. `git id --help` makes git run `man git-id` (only when `--help` directly follows `id`); `e2e/man_test.go` checks the requested page name with a recording viewer, so no `man` install is needed.
- Cleanup and signatures: git filter-repo drops the signature of every commit it rewrites, signed commits of other authors included (their hashes change); `cleanup` counts signed commits (`cleanup.CountSigned`, raw log, no verification) and warns. Checked on a 30000 commit history: about 2 s.
- CI: `.github/workflows/ci.yml` (fmt, vet, tests with all shells installed, cross-compile matrix) and `release.yml` (tag `v*` runs `make release` and uploads `dist/*`); YAML validated, never run on a runner. `README.md` is the user documentation, `LICENSE` is MIT.

# Design intent (from idea.md)
`git-id`: git identity tool. Problem: one user, multiple names, email addresses and SSH keys across different git contexts.

- Binary named `git-id` so git runs it as subcommand `git id`.
- Address book holds datapoints: names, email addresses, SSH keys. An *identity* = one name + one email + one SSH key.
- Identities reference datapoints, not copy them: editing a name/email/key propagates to every identity using it.
- `init` command: global setup. Takes existing global git user name and email, removes them from global config, moves them into the address book, and sets `user.useConfigOnly = true` so git never guesses an author for a repository without an identity.
- Shell hook (built, see Layout): on entering a directory with `.git`, check whether an identity is set; if not, tell the user to set one.
- CLI: intuitive command language, generated shell autocompletion covering all datapoints.
- TUI: choose an identity, manage datapoints and identities.
- Applying an identity: name/email = repo-local git config. SSH key selection undecided; leading idea is setting `GIT_SSH_COMMAND` (alternative worth weighing: `core.sshCommand` in repo config, which needs no env hook).
