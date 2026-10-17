# git-id

A git identity tool. If you commit as different people (work, private, a
client) you need a different name, email address and ssh key per repository.
git-id keeps them in an address book and applies the right combination to each
repository.

It installs as `git-id`, so git runs it as `git id`.

- An **address book** holds names, email addresses and ssh keys, each under a label.
- An **identity** combines one name, one email and one key by label. Change a
  name, email or key and every identity using it follows.
- `git id use <identity>` in a repository sets `user.name`, `user.email` and a
  `core.sshCommand` that makes git use that identity's key, only for that
  repository.

## Install

Download an archive from the releases (Linux, macOS, Windows; amd64 and arm64),
or build it:

    make install            # /usr/local/bin/git-id and its man pages
    make install PREFIX=~/.local
    make uninstall                  # removes both again (same PREFIX)

Needs Go to build; the binary has no runtime dependencies besides `git` and
`ssh`. `git id cleanup` additionally needs
[git-filter-repo](https://github.com/newren/git-filter-repo) (not bundled).

## Quick start

    git id init                                    # move the global name/email into the address book
    git id key add work ~/.ssh/id_work             # more datapoints
    git id email add work-mail me@company.example
    git id add work --name default --email work-mail --key work

    cd ~/src/company-project
    git id use work                                # this repository now commits and pushes as "work"
    git id current                                 # -> work

`init` imports the global `user.name` and `user.email` as the identity
`default`, using the first of `~/.ssh/id_ed25519`, `id_ecdsa`, `id_rsa`, and
removes both from the global git config and sets `user.useConfigOnly = true`, so that git cannot guess an author from the host name and a repository without an
identity cannot commit under the wrong name by accident.

## Commands

| | |
|---|---|
| `git id name\|email\|key add\|edit\|rename\|rm\|ls` | manage the datapoints (`key add <label> <private-key> [--public <file>]`) |
| `git id add\|edit\|rename\|rm\|ls\|show` | manage identities |
| `git id use <identity>` / `unuse` / `current` | apply, remove or show the identity of the current repository |
| `git id sync` | re-apply identities to every repository that used one, after you edited the address book |
| `git id repos ls\|forget` | the repositories `sync` tracks |
| `git id tui` | everything above interactively; `/` filters the list, editing offers the label for renaming, and in the key form `ctrl+o` lists the private keys in `~/.ssh` (and two levels below, passphrase protected ones marked) to choose from |
| `git id cleanup` | rewrite history written under the wrong identity (see below) |
| `git id hook`, `completion` | shell integration |

`git id --help` and the command help are coloured on a terminal (commands and flags you type in bold cyan, headings bold); `NO_COLOR=1` turns colours off, and piped output has none. `ls` and `show` take `--json`. Deleting a datapoint that identities still use
is refused unless you pass `--force`, which removes those identities too.
`git id --help` shows the manual page, `git-id <command> --help` the help of a command.

## How the key is chosen

`use` writes `core.sshCommand = git id ssh <identity>` into the repository. On
every push and fetch git runs that wrapper, which looks the key up in the
address book and runs `ssh -i <key> -o IdentitiesOnly=yes ...`. Editing a key
therefore takes effect immediately, without `sync`; names and emails are plain
repository config and need `sync`.

If your address book is not at the default location
(`$XDG_CONFIG_HOME/git-id/book.toml`, chosen with `--book` or `$GIT_ID_BOOK`),
its path is pinned in that command.

**Caveat:** git prefers the environment variable `GIT_SSH_COMMAND` over
`core.sshCommand`. If you have it exported, git-id's key selection is
bypassed. `use`, `current`, the TUI and the shell hook warn about it.

## Shell integration

    eval "$(git-id hook bash)"          # or zsh; fish: git-id hook fish | source
    source <(git-id completion bash)    # zsh/fish/powershell: see git-id completion --help

The hook reminds you when you enter a repository that has no identity, at its root or in any subdirectory, once per entry (moving around inside the repository is silent). The reminder is bold bright red and the `$GIT_SSH_COMMAND` warning bold bright yellow on a terminal (`NO_COLOR=1` switches that off). It finds the repository by looking for `.git` in the current directory and its parents and never starts git.
Completion knows the labels of your address book and also works for `git id <TAB>`
in bash, zsh and fish.

## cleanup: fix history written under the wrong identity

    git id cleanup --dry-run     # show what would change
    git id cleanup               # ask per value, then rewrite

It finds names and emails in the history that are in your address book but do
not belong to the identity of this repository, and offers to replace them with
the current identity's. You answer per value (yes, no, all remaining names or
emails, skip the rest, quit). Values that are not in your address book are never
touched.

**This rewrites history.** Commit hashes change, signatures of rewritten commits
are dropped, and by default all branches, tags and remote-tracking refs are
rewritten (`--current-branch` limits it). It refuses to run with uncommitted
changes, writes a bundle of all refs into `.git` first, prints the command that
restores it, asks you to type `yes`, and never pushes. Publishing the result
needs a force push and affects everyone who has the old history.

## Development

    make build      # bin/git-id
    make test       # go test ./... (the end-to-end tests need git, ssh; some zsh, fish, bash-completion, git-filter-repo)
    make release    # cross-built archives in dist/
    make man        # man pages in bin/man

See `CLAUDE.md` for the architecture and `idea.md` for the original design.

## License

MIT, see `LICENSE`.
