# Demo tape for github. Rendered with ascii-gif (github.com/tamnd/ascii-gif),
# which supplies the window chrome and theme; this file is just the action.
#
#   ascii-gif render docs/demo/github.tape -o docs/static/demo.gif
#
# github must be on PATH inside the recording shell, and github.com reachable.

Hide
Type "export PS1='$ ' PATH=$HOME/bin:$PATH && cd $(mktemp -d) && clear"
Enter
Show

Sleep 800ms
Type "github repo gohugoio/hugo --fields id,stars,forks,language"
Sleep 700ms
Enter
Sleep 3.5s

Type "clear"
Enter
Sleep 300ms
Type "github owned torvalds --fields id,stars,forks,language -n 5"
Sleep 700ms
Enter
Sleep 4s

Type "clear"
Enter
Sleep 300ms
Type "github deps gohugoio/hugo --fields package,version,ecosystem,license -n 6"
Sleep 700ms
Enter
Sleep 4s

Type "clear"
Enter
Sleep 300ms
Type "github edges golang/go --fields predicate,object,source -n 6"
Sleep 700ms
Enter
Sleep 4s

Type "clear"
Enter
Sleep 300ms
Type "github rdf gohugoio/hugo --format ttl | grep -m5 schema:"
Sleep 700ms
Enter
Sleep 4.5s
