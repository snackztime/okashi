# okashi demo tape — records the README GIF.
#
#   brew install vhs        # one-time (pulls ttyd + ffmpeg)
#   vhs demo.tape           # writes docs/demo.gif
#
# The tape builds okashi, copies the sample manuscript to a throwaway /tmp dir
# (so typing in the demo never dirties the committed corpus), then drives a short
# tour: open a project, flip the pane into the CORKBOARD (synopsis cards) and its
# full-screen spread, open a chapter and write a line, show the F1 help, and finish
# on the Tufte book preview. Tweak the Sleep values or navigation keys below if your
# terminal's timing/layout differs.

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
Type `export PATH="/opt/homebrew/bin:$PATH"; go build -o /tmp/okashi-demo-bin . && rm -rf /tmp/okashi-demo-corpus /tmp/okashi-demo-home && cp -r demo /tmp/okashi-demo-corpus && mkdir -p /tmp/okashi-demo-home`
Enter
Sleep 7s
# Stage the env here (not in the visible launch line) so the recording opens on a short command.
# OKASHI_ICONS=plain avoids Nerd-Font tofu in the recording font.
Type `export HOME=/tmp/okashi-demo-home OKASHI_ICONS=plain OKASHI_THEME=dark OKASHI_DIR=/tmp/okashi-demo-corpus; clear`
Enter
Sleep 1s
Show

# --- launch okashi on the sample manuscript ---
Type "/tmp/okashi-demo-bin"
Enter
Sleep 2.5s

# The launch hub focuses the LIBRARY on "The Lighthouse" — open it.
Enter
Sleep 1.5s

# Step onto the first chapter (row 0 is the ".." up-entry).
Down
Sleep 0.8s

# --- the CORKBOARD: the manuscript navigator ---
# ctrl+k flips the left pane from the chapter list to synopsis cards.
Ctrl+K
Sleep 3s

# `c` opens the full-screen corkboard — the roomy spread (J/K reorder, e synopsis).
Type "c"
Sleep 3.5s

# esc back to the pane; ctrl+k returns it to the compact chapter list.
Escape
Sleep 1s
Ctrl+K
Sleep 1s

# --- the help overlay (categorized shortcuts) ---
# `?` opens it from the sidebar (F1 works too in the app; VHS 0.11 has no F1 key).
Type "?"
Sleep 3.5s
Escape
Sleep 1s

# --- write (full-focus, dimmed) ---
# Open "The Keeper", collapse the file panel for a full-width surface, then add two lines.
# The first sentence dims once the cursor moves to the second — okashi's sentence-focus dimming.
Down
Sleep 0.5s
Enter
Sleep 1.2s
Ctrl+B
Sleep 1.5s
Type "The sea kept its own accounts, and the light was how a keeper settled them."
Sleep 1.5s
Enter
Type "He did not hurry."
Sleep 2.5s

# --- the Tufte book preview ---
# ctrl+p preview, then `t` for the warm Tufte view; scroll to the endnotes.
Ctrl+P
Sleep 2s
Type "t"
Sleep 2.5s
Down
Down
Down
Sleep 3s

# Back to editing, then quit.
Ctrl+P
Sleep 1.5s
Ctrl+C
Sleep 1s
