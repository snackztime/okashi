package main

import "github.com/charmbracelet/lipgloss"

// sidebarWidth is the fixed width of the file pane (including its border).
const sidebarWidth = 34

// The Crush-ish palette. Tweak freely — this is where the "vibe" lives.
var (
	accent = lipgloss.Color("#7D56F4") // Charm purple
	subtle = lipgloss.Color("#6C6C6C")
)

// Per-type icon colors (Dracula palette). The base palette above is unchanged.
var (
	iconFolderColor  = lipgloss.Color("#8be9fd") // cyan
	iconParentColor  = lipgloss.Color("#6272a4") // comment grey
	iconTextColor    = lipgloss.Color("#f8f8f2") // foreground
	iconPdfColor     = lipgloss.Color("#ff5555") // red
	iconImageColor   = lipgloss.Color("#50fa7b") // green
	iconCodeColor    = lipgloss.Color("#f1fa8c") // yellow
	iconGenericColor = lipgloss.Color("#6272a4") // comment grey
)

var bannerStyle = lipgloss.NewStyle().
	Foreground(accent).
	Bold(true)

var inspectorStyle = lipgloss.NewStyle().
	Border(lipgloss.RoundedBorder(), false, false, false, true).
	BorderForeground(subtle).
	Padding(0, 1)

var statusStyle = lipgloss.NewStyle().
	Foreground(subtle).
	Padding(0, 1)

var selectedStyle = lipgloss.NewStyle().
	Foreground(lipgloss.Color("#FFFFFF")).
	Background(accent)

var breadcrumbStyle = lipgloss.NewStyle().
	Foreground(accent).
	Bold(true)

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
