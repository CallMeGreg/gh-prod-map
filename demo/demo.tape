# VHS tape for the gh-prod-map demo.
#
# Prerequisites:
#   - Build and install the extension locally so `gh prod-map` resolves:
#       go build -o gh-prod-map .
#       gh extension install .
#   - Install VHS: https://github.com/charmbracelet/vhs
#
# Record the GIF with:
#   vhs demo/demo.tape

Output demo/demo.gif

Set Shell "zsh"
Set FontSize 18
Set Width 1200
Set Height 720
Set Padding 20
Set Theme "Dracula"

# Start from a clean, minimal prompt.
Hide
Type "export PS1='$ '"
Enter
Type "clear"
Enter
Show

Type "gh prod-map --help"
Sleep 750ms
Enter
Sleep 4s

Type "gh prod-map repos --org cli --limit 5"
Sleep 750ms
Enter
Sleep 6s
