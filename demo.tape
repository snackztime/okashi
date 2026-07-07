# okashi demo tape — records the README GIF.
#
#   brew install vhs        # one-time (pulls ttyd + ffmpeg)
#   vhs demo.tape           # writes docs/demo.gif
#
# The tape builds okashi, copies the sample manuscript to a throwaway /tmp dir
# (so the demo never dirties the committed corpus), then drives a short tour: open a
# project, the full-screen CORKBOARD (ctrl+k), the OUTLINE (ctrl+l), the F1 help,
# writing with sentence-focus dimming, and the Tufte book preview. Tweak the Sleep
# values or navigation keys below if your terminal's timing/layout differs.

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

# The launch hub focuses the LIBRARY on the sample — open it.
Enter
Sleep 1.2s

# Open the first chapter, then step back to the chapter list (so ctrl+l has a chapter to return to).
Down
Sleep 0.4s
Enter
Sleep 1.2s
Escape
Sleep 0.8s

# --- the CORKBOARD (ctrl+k): the full-screen manuscript navigator ---
# Synopsis cards + a progress header (chapters · words · goal); J/K or shift+↑↓ reorder.
Ctrl+K
Sleep 3.5s
Escape
Sleep 1s

# --- the OUTLINE (ctrl+l): two-level beats + notes; a checked beat has been promoted to a chapter ---
Ctrl+L
Sleep 3.5s
Escape
Sleep 0.8s

# --- the help overlay (categorized shortcuts) ---
# esc from the editor returns to the sidebar; `?` opens help there (F1 works too).
Escape
Sleep 0.4s
Type "?"
Sleep 3s
Escape
Sleep 0.8s

# --- write (full-focus, sentence dimming) ---
# Open a chapter, collapse the file panel for a full-width surface, then add two lines.
# The first sentence dims once the cursor moves to the second — okashi's sentence-focus dimming.
Down
Sleep 0.4s
Enter
Sleep 1s
Ctrl+B
Sleep 1.2s
Type "The sea kept its own accounts, and the light was how a keeper settled them."
Sleep 1.2s
Enter
Type "He did not hurry."
Sleep 2.5s

# --- the Tufte book preview ---
# ctrl+p preview, then `t` for the warm Tufte view; scroll to the endnotes.
Ctrl+P
Sleep 1.8s
Type "t"
Sleep 2.5s
Down
Down
Down
Sleep 2.5s

# Back to editing, then quit.
Ctrl+P
Sleep 1.2s
Ctrl+C
Sleep 1s
