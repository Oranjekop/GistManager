package app

import (
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
)

type palette struct{ page, pageTop, panel, card, muted, input, border, text, soft, secondary, accent, strong, success, danger ui.Color }

// These values come from the Electron client's CSS variables.
var lightPalette = palette{
	page: ui.Hex("#f3f6fb"), pageTop: ui.Hex("#f8fbff"), panel: ui.RGBA(255, 255, 255, .84),
	card: ui.RGBA(248, 250, 252, .96), muted: ui.Hex("#f7f9fc"), input: ui.RGBA(255, 255, 255, .92),
	border: ui.RGBA(15, 23, 42, .1), text: ui.Hex("#0f172a"), soft: ui.Hex("#334155"), secondary: ui.Hex("#64748b"),
	accent: ui.Hex("#2563eb"), strong: ui.Hex("#1d4ed8"), success: ui.Hex("#15803d"), danger: ui.Hex("#dc2626"),
}
var darkPalette = palette{
	page: ui.Hex("#060c16"), pageTop: ui.Hex("#0a1423"), panel: ui.RGBA(10, 19, 34, .94),
	card: ui.RGBA(14, 27, 46, .94), muted: ui.RGBA(19, 35, 57, .88), input: ui.RGBA(5, 13, 25, .94),
	border: ui.RGBA(148, 163, 184, .14), text: ui.Hex("#d7deea"), soft: ui.Hex("#cbd5e1"), secondary: ui.Hex("#94a3b8"),
	accent: ui.Hex("#38bdf8"), strong: ui.Hex("#0ea5e9"), success: ui.Hex("#4ade80"), danger: ui.Hex("#fb7185"),
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
	t.SurfaceHover, t.SurfacePressed = p.muted.Mix(p.accent, .06), p.muted.Mix(p.accent, .1)
	t.Success, t.Danger, t.Focus, t.Selection = p.success, p.danger, p.accent.Alpha(.55), p.accent.Alpha(.25)
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
