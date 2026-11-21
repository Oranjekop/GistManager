package app

import (
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
)

type palette struct{ page, pageTop, panel, card, muted, input, border, text, soft, secondary, accent, strong, success, danger ui.Color }

// Neutral surfaces and high-contrast controls for both appearances.
var lightPalette = palette{
	page: ui.Hex("#f3f3f3"), pageTop: ui.Hex("#fafafa"), panel: ui.RGBA(255, 255, 255, .84),
	card: ui.RGBA(248, 248, 248, .96), muted: ui.Hex("#f1f1f1"), input: ui.RGBA(255, 255, 255, .92),
	border: ui.RGBA(0, 0, 0, .1), text: ui.Hex("#191919"), soft: ui.Hex("#454545"), secondary: ui.Hex("#666666"),
	accent: ui.Hex("#292929"), strong: ui.Hex("#414141"), success: ui.Hex("#444444"), danger: ui.Hex("#111111"),
}
var darkPalette = palette{
	page: ui.Hex("#191919"), pageTop: ui.Hex("#242424"), panel: ui.RGBA(32, 32, 32, .94),
	card: ui.RGBA(40, 40, 40, .94), muted: ui.RGBA(46, 46, 46, .88), input: ui.RGBA(25, 25, 25, .94),
	border: ui.RGBA(255, 255, 255, .14), text: ui.Hex("#eeeeee"), soft: ui.Hex("#d8d8d8"), secondary: ui.Hex("#ababab"),
	accent: ui.Hex("#d9d9d9"), strong: ui.Hex("#ededed"), success: ui.Hex("#c9c9c9"), danger: ui.Hex("#ffffff"),
}

func colors(dark bool) palette {
	if dark {
		return darkPalette
	}
	return lightPalette
}

func legacyInput(c *ui.Context, value *string) ui.Element {
	return ui.TextInput(c, value).Background(colors(c.Theme().Dark).input)
}

func legacyTheme(t ui.Theme) ui.Theme {
	p := colors(t.Dark)
	t.Background, t.Surface, t.Border, t.Text, t.TextMuted = p.page, p.muted, p.border, p.text, p.secondary
	t.Accent, t.AccentHover, t.AccentPressed, t.AccentText = p.accent, p.strong, p.strong, ui.Hex("#ffffff")
	if t.Dark {
		t.AccentText = ui.Hex("#171717")
		t.AccentPressed = ui.Hex("#bdbdbd")
	}
	t.Warning, t.Scrollbar = p.danger, p.secondary.Alpha(.65)
	t.SurfaceHover, t.SurfacePressed = p.muted.Mix(p.accent, .06), p.muted.Mix(p.accent, .1)
	t.Success, t.Danger, t.Focus, t.Selection = p.success, p.danger, p.accent.Alpha(.75), p.accent.Alpha(.25)
	return t
}

var codeTheme = func() *editor.Theme {
	t := editor.DefaultTheme(ui.DarkTheme())
	t.Background, t.Text = ui.Hex("#0f172a"), ui.Hex("#d4d8e3")
	t.LineNumber, t.ActiveLineNumber, t.Cursor = ui.Hex("#475569"), ui.Hex("#cbd5e1"), ui.Hex("#f8fafc")
	t.Selection, t.InactiveSelection = ui.Hex("#1d4ed866"), ui.Hex("#1e293b")
	t.IndentGuide, t.ActiveIndentGuide = ui.Hex("#172033"), ui.Hex("#334155")
	t.ActiveLine = ui.Hex("#141e33")
	for _, entry := range []struct {
		kind  editor.TokenKind
		color string
	}{
		{editor.TokenKeyword, "#7dd3fc"}, {editor.TokenComment, "#64748b"}, {editor.TokenString, "#f9a8d4"}, {editor.TokenNumber, "#fcd34d"},
		{editor.TokenFunction, "#dcdcaa"}, {editor.TokenType, "#4ec9b0"}, {editor.TokenOperator, "#d4d8e3"}, {editor.TokenProperty, "#9cdcfe"},
	} {
		t.Tokens[entry.kind].Color = ui.Hex(entry.color)
	}
	return t
}()
