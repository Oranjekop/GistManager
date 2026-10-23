package app

import (
	"context"
	"fmt"
	"image"
	"math"
	"strings"
	"testing"

	"github.com/Oranjekop/GistManager/internal/gist"
	"github.com/Oranjekop/GistManager/internal/security"
	"github.com/egoist/mygo/ui"
)

func TestGistCardTextStaysInItsRow(t *testing.T) {
	for _, sidebar := range []float32{240, 300, 380} {
		t.Run(fmt.Sprintf("sidebar-%g", sidebar), func(t *testing.T) {
			a := fixtureApp(t)
			a.sidebarWidth = sidebar
			a.gists[0].Description = "Layout fixture"
			a.gists[0].Files = map[string]gist.File{"short.json": {Filename: "short.json", Content: "{}"}}
			tt := ui.NewTester(a.View, 1500, 940)
			card := findBox(t, tt, "Layout fixture")
			filename := findBox(t, tt, "short.json")
			baseline := copyFrame(tt)

			for _, title := range []string{
				"Fanqie Writer encrypted backup: " + strings.Repeat("abcdef0123456789", 4),
				"UniGetUI package backups - DO NOT RENAME OR MODIFY (UNIGETUI_BACKUP_V1)",
				"多行描述\n第二行描述\n第三行描述",
			} {
				a.gists[0].Description = title
				tt.Frame()
				assertPixelsUnchanged(t, baseline, tt.Image(), image.Rect(
					int(card.X)+1, int(math.Ceil(float64(filename.Y))),
					int(card.X+card.W)-1, int(card.Y+card.H)-1,
				), "title painted over the filename or metadata")
			}

			a.gists[0].Description = "Layout fixture"
			longFilename := "fanqie-sync-backup-" + strings.Repeat("abcdef0123456789", 4) + ".json"
			a.gists[0].Files = map[string]gist.File{longFilename: {Filename: longFilename, Content: "{}"}}
			tt.Frame()
			assertPixelsUnchanged(t, baseline, tt.Image(), image.Rect(
				int(card.X)+1, int(math.Ceil(float64(filename.Y+filename.H))),
				int(card.X+card.W)-1, int(card.Y+card.H)-1,
			), "filename painted over the metadata")
		})
	}
}

func TestGistHoverCoversTheCardAndPreservesRoundedCorners(t *testing.T) {
	for _, dark := range []bool{false, true} {
		for _, sidebar := range []float32{240, 300, 380} {
			t.Run(fmt.Sprintf("dark-%t-width-%g", dark, sidebar), func(t *testing.T) {
				a := fixtureApp(t)
				a.sidebarWidth = sidebar
				tt := ui.NewTester(a.View, 1500, 940)
				tt.SetDark(dark)
				selected := findBox(t, tt, "Gist 卡片 demo-1")
				card := findBox(t, tt, "Gist 卡片 demo-2")
				baseline := copyFrame(tt)
				tt.Move(card.X+card.W/2, card.Y+card.H/2)
				hovered := tt.Image()
				for _, point := range [][2]float32{
					{card.X + card.W/2, card.Y + 3},
					{card.X + card.W/2, card.Y + card.H - 4},
					{card.X + 3, card.Y + card.H/2},
					{card.X + card.W - 4, card.Y + card.H/2},
				} {
					x, y := int(point[0]), int(point[1])
					if baseline.RGBAAt(x, y) == hovered.RGBAAt(x, y) {
						t.Fatalf("hover did not reach the card edge at (%d, %d)", x, y)
					}
				}
				for _, x := range []float32{card.X + 2, card.X + card.W - 3} {
					for _, y := range []float32{card.Y + 2, card.Y + card.H - 3} {
						if baseline.RGBAAt(int(x), int(y)) != hovered.RGBAAt(int(x), int(y)) {
							t.Fatal("list hover leaked outside a rounded card corner", x, y)
						}
					}
				}
				assertPixelsUnchanged(t, baseline, hovered, image.Rect(
					int(selected.X), int(selected.Y), int(selected.X+selected.W), int(selected.Y+selected.H),
				), "hovering another card changed the selected card")
				snapshot(t, tt, fmt.Sprintf("gist-hover-%g-dark-%t.png", sidebar, dark))
				tt.Move(1490, 20)
				assertPixelsUnchanged(t, baseline, tt.Image(), image.Rect(
					int(card.X), int(card.Y), int(card.X+card.W), int(card.Y+card.H),
				), "card hover remained after the pointer left")
				click(t, tt, a.gists[0].Title())
				tt.Key(0, ui.KeyDown)
				if a.current.ID != "demo-2" {
					t.Fatal("card styling broke keyboard selection", a.current.ID)
				}
			})
		}
	}
}

func TestSettingsDialogHasEqualSideInsets(t *testing.T) {
	for _, password := range []bool{false, true} {
		t.Run(fmt.Sprintf("password-%t", password), func(t *testing.T) {
			a := fixtureApp(t)
			if password {
				if err := a.Security.Configure(context.Background(), security.Options{NewPassword: "fixture-password"}); err != nil {
					t.Fatal(err)
				}
			}
			a.openSettings()
			tt := ui.NewTester(a.View, 1500, 940)
			for _, size := range [][2]int{{1500, 940}, {1000, 680}} {
				tt.SetSize(size[0], size[1])
				panel := findBox(t, tt, "系统设置对话框")
				field := findBox(t, tt, "程序密码设置")
				save := findBox(t, tt, "保存设置")
				left, right := field.X-panel.X, panel.X+panel.W-field.X-field.W
				if math.Abs(float64(left-right)) > 1 || left < 16 {
					t.Fatalf("unequal dialog insets at %v: left=%g right=%g", size, left, right)
				}
				if save.X < field.X || save.X+save.W > field.X+field.W {
					t.Fatalf("footer action outside the form width: field=%+v save=%+v", field, save)
				}
				if panel.X < 0 || panel.Y < 0 || panel.X+panel.W > float32(size[0]) || panel.Y+panel.H > float32(size[1]) {
					t.Fatalf("dialog outside window %v: %+v", size, panel)
				}
			}
			snapshot(t, tt, fmt.Sprintf("settings-password-%t-light.png", password))
			tt.SetDark(true)
			snapshot(t, tt, fmt.Sprintf("settings-password-%t-dark.png", password))
		})
	}
}

func TestLongContentLayoutAndSnapshots(t *testing.T) {
	a := fixtureApp(t)
	base := clone(a.gists[0])
	filename := "fanqie-sync-backup-" + strings.Repeat("abcdef0123456789", 3) + ".json"
	a.gists = nil
	for i, title := range []string{
		"Fanqie Writer encrypted backup: " + strings.Repeat("abcdef0123456789", 3),
		"Netcatty Encrypted Vault (DO NOT EDIT MANUALLY)",
		"UniGetUI package backups - DO NOT RENAME OR MODIFY (UNIGETUI_BACKUP_V1)",
		"多行 Gist 描述\n第二行描述\n第三行描述",
	} {
		g := clone(base)
		g.ID, g.Description = fmt.Sprintf("layout-%d", i), title
		g.Files = map[string]gist.File{filename: {Filename: filename, Content: `{"fixture": true}`}}
		if i == 2 {
			for j := 0; j < 10; j++ {
				name := fmt.Sprintf("@PACKAGEST-layout-backup-%d.json", j)
				g.Files[name] = gist.File{Filename: name, Content: "{}"}
			}
		}
		a.gists = append(a.gists, g)
	}
	a.showGist(a.gists[0], filename)
	a.user.Login = strings.Repeat("layout", 12)
	a.status = strings.Repeat("长状态消息与错误详情 ", 30)
	for _, size := range [][2]int{{1500, 940}, {1000, 680}} {
		tt := ui.NewTester(a.View, size[0], size[1])
		for _, label := range []string{"系统设置", "编辑", "复制 Raw", "自动换行"} {
			box := findBox(t, tt, label)
			if box.W <= 0 || box.H <= 0 || box.X < 0 || box.X+box.W > float32(size[0]) || box.Y+box.H > float32(size[1]) {
				t.Fatalf("%s outside window %v: %+v", label, size, box)
			}
		}
		snapshot(t, tt, fmt.Sprintf("long-content-%d-light.png", size[0]))
		tt.SetDark(true)
		snapshot(t, tt, fmt.Sprintf("long-content-%d-dark.png", size[0]))
	}
}

func findBox(t *testing.T, tt *ui.Tester, label string) ui.Rect {
	t.Helper()
	box, found := tt.Find(label)
	if !found {
		t.Fatalf("missing UI element %q", label)
	}
	return box
}

func copyFrame(tt *ui.Tester) *image.RGBA {
	frame := *tt.Image()
	frame.Pix = append([]byte(nil), frame.Pix...)
	return &frame
}

func assertPixelsUnchanged(t *testing.T, before, after *image.RGBA, area image.Rectangle, message string) {
	t.Helper()
	area = area.Intersect(before.Bounds()).Intersect(after.Bounds())
	if area.Empty() {
		t.Fatal("empty pixel comparison area")
	}
	for y := area.Min.Y; y < area.Max.Y; y++ {
		for x := area.Min.X; x < area.Max.X; x++ {
			if before.RGBAAt(x, y) != after.RGBAAt(x, y) {
				t.Fatalf("%s at (%d, %d)", message, x, y)
			}
		}
	}
}
