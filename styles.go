package main

import "github.com/charmbracelet/lipgloss"

// sidebarWidth is the fixed width of the file pane (including its border).
const sidebarWidth = 32

// The Crush-ish palette. Tweak freely — this is where the "vibe" lives.
var (
	accent = lipgloss.Color("#7D56F4") // Charm purple
	subtle = lipgloss.Color("#6C6C6C")
)

var bannerStyle = lipgloss.NewStyle().
	Foreground(accent).
	Bold(true)

var sidebarStyle = lipgloss.NewStyle().
	Border(lipgloss.RoundedBorder(), false, true, false, false).
	BorderForeground(subtle).
	Padding(0, 1)

var statusStyle = lipgloss.NewStyle().
	Foreground(subtle).
	Padding(0, 1)

var selectedStyle = lipgloss.NewStyle().
	Foreground(lipgloss.Color("#FFFFFF")).
	Background(accent)

// bannerArt is figlet's "small" font. Regenerate for your app's real name:
//
//	figlet -f small YOURNAME
//
// then paste the output between the backticks.
const bannerArt = `     _           _    _
 ___| |____ _ __| |_ (_)
/ _ \ / / _` + "`" + ` (_-< ' \| |
\___/_\_\__,_/__/_||_|_|`

// bannerView styles the ASCII art and centers it across the window.
func bannerView(width int) string {
	return lipgloss.PlaceHorizontal(width, lipgloss.Center, bannerStyle.Render(bannerArt))
}
