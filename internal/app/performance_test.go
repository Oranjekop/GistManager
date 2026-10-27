package app

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

func BenchmarkWorkspace(b *testing.B) {
	for _, dark := range []bool{false, true} {
		for _, scroll := range []bool{false, true} {
			b.Run(fmt.Sprintf("dark-%t/scroll-%t", dark, scroll), func(b *testing.B) {
				a := fixtureApp(b)
				a.wrap = false
				a.resetEditor(strings.Repeat("// workspace benchmark "+strings.Repeat("x", 120)+"\n", 1000))
				base := clone(a.gists[1])
				for i := 0; i < 200; i++ {
					item := clone(base)
					item.ID = fmt.Sprintf("benchmark-%d", i)
					a.gists = append(a.gists, item)
				}
				tt := ui.NewTester(a.View, 1500, 940)
				tt.SetDark(dark)
				box, ok := tt.Find("代码编辑器")
				if !ok {
					b.Fatal("editor is missing")
				}
				x, y := box.X+box.W/2, box.Y+box.H/2
				tt.Scroll(x, y, 0, -5000)
				tt.Advance(250 * time.Millisecond)
				for i := 0; i < 5; i++ {
					tt.Frame()
				}
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					if scroll {
						delta := float32(80)
						if i%2 != 0 {
							delta = -delta
						}
						tt.Scroll(x, y, 0, delta)
					} else {
						tt.Frame()
					}
				}
			})
		}
	}
}
