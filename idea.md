 #Problem
working with git in multiple different situations, which brings the problem, that i need to represent myself with 
 different names, emailaddresses and need to authenticate with different ssh keys.
 
# solution
git-id, a git identity tool.

the binary will be named git-id, so that it can be a git-sub-command be called by `git id`.

it should mage a addressbook, containing names, emailaddresses and ssh keys.

a set of a name an emailaddress and a ssh key will be called a identity.

once a name, emailaddress and ssh key changes, all identities automagicaly represent these changes.

git id should have a init command that globally initialized itself.

it will grab the global and email-address, remove it and transfer it to the address book.

we need to figure out how, but somehow, some shell hook properly, we must, once we enter a directory containing a .git directory check if an identity is set, and if not tell the user ne should do so.

git-id should have an intuitive language model for the commandline and have autocompletion-generation, including all datapoints.

git-id also should have a tui for choosing an identity and for datapoint and identity management.

setting name and email-address is easy, thats just a repo-wide config, but somehow we need to convince git which key to use, most likely we set GIT_SSH_COMMAND?


# planned last: git id cleanup

`git id cleanup` fixes history that was committed with the wrong identity.

it scans the git history of the current repository for author and committer names and email addresses that are known in the address book but do not belong to the identity currently active in this repository (see `git id current`). names and addresses unknown to the address book are never touched.

for every such finding it asks interactively whether it should be replaced by the name/email of the current identity. the user can answer

- one by one, per distinct name/email value found, showing how many commits are affected,
- or batched, per name or per email address ("replace all commits by this name", "this address"), so that one answer covers every commit carrying that value.

decisions (implemented):

- rewriting uses `git filter-repo` with name and email callbacks (`--partial --force`, so the origin remote, remote-tracking refs and the reflog stay). it is an external tool and is not bundled: if it is missing, git-id says so and tells the user to install it.
- author and committer are always handled together (and tagger names/emails of annotated tags get the same replacement).
- scope: ALL refs by default, and the command is loud about it (banner before any question). `--current-branch` limits it to the checked-out branch.
- answers per finding: yes / no / all remaining of this kind / skip remaining of this kind / quit. "all" and "skip" are the batch by name or by address. `--yes` accepts everything and skips the confirmation, `--dry-run` only reports.
- safety: refuses on uncommitted changes to tracked files, shows how many commits change and how many of them are already in remote-tracking refs (force push needed), requires typing the word "yes", writes a bundle of all refs into the git directory first and prints the command that restores it. nothing is ever pushed.
- known limits: commit messages and trailers (`Signed-off-by`, `Co-authored-by`) are not touched; signatures of rewritten commits are lost; email matching ignores case, names match exactly.
