# okashi demo tape — records the README GIF.
#
#   brew install vhs        # one-time (pulls ttyd + ffmpeg)
#   vhs demo.tape           # writes docs/demo.gif
#
# The tape builds okashi, copies the sample manuscript to a throwaway /tmp dir
# (so typing in the demo never dirties the committed corpus), then drives a short
# tour: open a project, open a chapter, type a line, and toggle the Tufte preview
# to show margin sidenotes. Tweak the Sleep values or the two navigation Enters
# below if your terminal's timing/layout differs.

Output docs/demo.gif

Set Shell "bash"
Set FontSize 16
Set Width 1400
Set Height 850
Set Padding 24
Set Theme "Catppuccin Mocha"

# --- silent setup: build, stage a scratch corpus, and an EMPTY config home ---
# VHS spawns a non-login shell, so put Homebrew (go) on PATH explicitly.
# The scratch HOME gives okashi empty recents/pins so the hub focuses the demo
# project (not your real files) — and keeps your writing out of the recording.
Hide
Type `export PATH="/opt/homebrew/bin:$PATH"; go build -o /tmp/okashi-demo-bin . && rm -rf /tmp/okashi-demo-corpus /tmp/okashi-demo-home && cp -r demo /tmp/okashi-demo-corpus && mkdir -p /tmp/okashi-demo-home && clear`
Enter
Sleep 6s
Show

# --- launch okashi on the sample manuscript ---
# OKASHI_ICONS=plain avoids Nerd-Font tofu in the recording font.
# HOME points at the scratch config dir; the prose measure stays 72; the wide
# window lets the Tufte preview show sidenotes.
Type "HOME=/tmp/okashi-demo-home OKASHI_ICONS=plain OKASHI_THEME=dark OKASHI_DIR=/tmp/okashi-demo-corpus /tmp/okashi-demo-bin"
Enter
Sleep 2.5s

# The launch hub focuses the LIBRARY on "The Lighthouse" — open it.
Enter
Sleep 1.5s

# The manuscript sidebar lists the chapters; row 0 is the ".." up-entry, so step
# down onto "The Keeper" before opening it into the editor.
Down
Sleep 0.8s
Enter
Sleep 2.5s

# Collapse the file panel for a full-width writing surface.
Ctrl+B
Sleep 2s

# Open the inspector and cycle its tabs: Words -> Outline -> Goals -> Analysis;
# a fifth Ctrl+Y closes it again.
Ctrl+Y
Sleep 2s
Ctrl+Y
Sleep 2s
Ctrl+Y
Sleep 2s
Ctrl+Y
Sleep 2s
Ctrl+Y
Sleep 1s

# Markdown preview, then the Tufte book view: the chapter's footnotes float into
# margin sidenotes (the pane widens; the prose stays at the 72-col measure).
Ctrl+P
Sleep 2.5s
Type "t"
Sleep 5s

# Back to editing, then quit.
Ctrl+P
Sleep 1.5s
Ctrl+C
Sleep 1s
