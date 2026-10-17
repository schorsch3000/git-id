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
