package l10n

import (
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"testing"

	"github.com/egoist/mygo/i18n"
)

func TestLocaleMatching(t *testing.T) {
	for _, locale := range []string{"zh-CN", "zh-SG", "zh-Hans", "zh-TW", "zh-HK", "en-US", "en-GB", "fr-FR"} {
		resolved := i18n.Match([]string{locale}, []string{"en", "zh", "zh-Hant"})
		if resolved == "" {
			resolved = "en"
		}
		want := "Settings"
		if locale[:2] == "zh" {
			want = "系统设置"
		}
		if got := In(resolved, "系统设置"); got != want {
			t.Errorf("%s resolved to %s: %s", locale, resolved, got)
		}
	}
}

func TestCatalogCoverageAndFormatArguments(t *testing.T) {
	verbs := regexp.MustCompile(`%[-+# 0-9.]*[a-zA-Z]`)
	for source, translated := range english {
		if !slices.Equal(verbs.FindAllString(source, -1), verbs.FindAllString(translated, -1)) {
			t.Errorf("format arguments changed: %q -> %q", source, translated)
		}
	}
	for _, dir := range []string{"../app", "../security", "../gist", "../update"} {
		files, err := filepath.Glob(filepath.Join(dir, "*.go"))
		if err != nil {
			t.Fatal(err)
		}
		for _, path := range files {
			file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
			if err != nil {
				t.Fatal(err)
			}
			ast.Inspect(file, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok || len(call.Args) == 0 {
					return true
				}
				fun, ok := call.Fun.(*ast.SelectorExpr)
				if !ok {
					return true
				}
				pkg, ok := fun.X.(*ast.Ident)
				if !ok || pkg.Name != "l10n" {
					return true
				}
				literal, ok := call.Args[0].(*ast.BasicLit)
				if !ok || literal.Kind != token.STRING {
					return true
				}
				key, _ := strconv.Unquote(literal.Value)
				if !regexp.MustCompile(`\p{Han}`).MatchString(key) {
					return true
				}
				if _, exists := english[key]; !exists {
					t.Errorf("missing English translation in %s: %s", path, key)
				}
				return true
			})
		}
	}
}

func TestErrorsResolveAtDisplayTimeAndPreserveCause(t *testing.T) {
	locale := "zh"
	i18n.SetLocaleFunc(func() string { return locale })
	t.Cleanup(func() { i18n.SetLocaleFunc(nil) })
	cause := errors.New("network timeout")
	err := Errorf("连接 GitHub 失败: %w", cause)
	sentinel := Error("应用尚未解锁")
	if sentinel.Error() != "应用尚未解锁" {
		t.Fatal(sentinel)
	}
	locale = "en"
	if sentinel.Error() != "App is locked" || err.Error() != "Could not connect to GitHub: network timeout" || !errors.Is(err, cause) {
		t.Fatal("localized errors lost the language or cause", err)
	}
}
