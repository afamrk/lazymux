package ui

import "github.com/charmbracelet/lipgloss"

// Catppuccin Mocha. The terminal's own background is kept (no global bg), so
// lazymux blends into whatever the user's terminal theme is.
const (
	cSurface0 = lipgloss.Color("#313244")
	cSurface1 = lipgloss.Color("#45475a")
	cSelDim   = lipgloss.Color("#2a2b3d") // selected row in an unfocused panel
	cOverlay0 = lipgloss.Color("#6c7086")
	cSubtext  = lipgloss.Color("#a6adc8")
	cText     = lipgloss.Color("#cdd6f4")
	cBlue     = lipgloss.Color("#89b4fa")
	cGreen    = lipgloss.Color("#a6e3a1")
	cYellow   = lipgloss.Color("#f9e2af")
	cRed      = lipgloss.Color("#f38ba8")
	cMauve    = lipgloss.Color("#cba6f7")
	cPeach    = lipgloss.Color("#fab387")
	cTeal     = lipgloss.Color("#94e2d5")
	cSky      = lipgloss.Color("#89dceb")

	// tinted backgrounds for chips and the blocked agent's card
	cRedBg    = lipgloss.Color("#3b2a35")
	cYellowBg = lipgloss.Color("#3a3631")
	cGreenBg  = lipgloss.Color("#2c3a32")
	cCursor   = lipgloss.Color("#3e4566") // the sidebar's cursor row
)

var (
	stText    = lipgloss.NewStyle().Foreground(cText)
	stSub     = lipgloss.NewStyle().Foreground(cSubtext)
	stFaint   = lipgloss.NewStyle().Foreground(cOverlay0)
	stBlue    = lipgloss.NewStyle().Foreground(cBlue)
	stGreen   = lipgloss.NewStyle().Foreground(cGreen)
	stRed     = lipgloss.NewStyle().Foreground(cRed)
	stYellow  = lipgloss.NewStyle().Foreground(cYellow)
	stMauve   = lipgloss.NewStyle().Foreground(cMauve)
	stPeach   = lipgloss.NewStyle().Foreground(cPeach)
	stTeal    = lipgloss.NewStyle().Foreground(cTeal)
	stKey     = lipgloss.NewStyle().Foreground(cBlue).Bold(true)
	stTitleOn = lipgloss.NewStyle().Foreground(cBlue).Bold(true)
)
