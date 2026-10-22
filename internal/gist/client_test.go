package gist

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGitHubCRUDAndPagination(t *testing.T) {
	var calls []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.Method+" "+r.URL.Path)
		if r.Header.Get("Authorization") != "Bearer fixture-token" || r.Header.Get("X-GitHub-Api-Version") == "" {
			t.Error("missing GitHub authentication headers")
		}
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/user":
			fmt.Fprint(w, `{"login":"alice"}`)
		case r.Method == "GET" && r.URL.Path == "/gists":
			items := []Gist{{ID: "101", Files: map[string]File{}}}
			if r.URL.Query().Get("page") == "1" {
				items = make([]Gist, 100)
				for i := range items {
					items[i].ID = fmt.Sprint(i)
				}
			}
			_ = json.NewEncoder(w).Encode(items)
		case r.Method == "GET":
			fmt.Fprint(w, `{"id":"101","files":{"main.go":{"filename":"main.go","content":"package main"}}}`)
		case r.Method == "DELETE":
			w.WriteHeader(204)
		default:
			var patch Patch
			if err := json.NewDecoder(r.Body).Decode(&patch); err != nil {
				t.Fatal(err)
			}
			if len(patch.Files) == 0 {
				t.Error("missing file operations")
			}
			if r.Method == "PATCH" && patch.Files["old.go"] != nil && patch.Files["old.go"].Filename != "new.go" {
				t.Error("rename lost")
			}
			fmt.Fprint(w, `{"id":"101","files":{"main.go":{"content":"saved"}}}`)
		}
	}))
	defer server.Close()
	c := &Client{HTTP: server.Client(), BaseURL: server.URL}
	ctx := context.Background()
	user, err := c.Session(ctx, "fixture-token")
	if err != nil || user.Login != "alice" {
		t.Fatalf("session: %+v %v", user, err)
	}
	items, err := c.List(ctx, "fixture-token")
	if err != nil || len(items) != 101 {
		t.Fatalf("pagination: %d %v", len(items), err)
	}
	g, err := c.Get(ctx, "fixture-token", "101")
	if err != nil || g.Files["main.go"].Content != "package main" {
		t.Fatal(g, err)
	}
	content, description := "saved", "fixture"
	if _, err = c.Create(ctx, "fixture-token", Patch{Description: &description, Files: map[string]*FileChange{"main.go": {Content: &content}}}); err != nil {
		t.Fatal(err)
	}
	if _, err = c.Update(ctx, "fixture-token", "101", Patch{Files: map[string]*FileChange{"old.go": {Filename: "new.go"}, "delete.go": nil}}); err != nil {
		t.Fatal(err)
	}
	if err = c.Delete(ctx, "fixture-token", "101"); err != nil {
		t.Fatal(err)
	}
	if len(calls) != 7 {
		t.Fatalf("unexpected requests: %v", calls)
	}
}

type transportFunc func(*http.Request) (*http.Response, error)

func (fn transportFunc) RoundTrip(r *http.Request) (*http.Response, error) { return fn(r) }

func TestRawContentNeverReceivesToken(t *testing.T) {
	c := NewClient()
	c.HTTP.Transport = transportFunc(func(r *http.Request) (*http.Response, error) {
		text := `{"id":"1","files":{"big.go":{"filename":"big.go","truncated":true,"raw_url":"https://gist.githubusercontent.com/alice/raw/big.go"}}}`
		if r.URL.Host == "gist.githubusercontent.com" {
			if r.Header.Get("Authorization") != "" {
				t.Fatal("leaked API token to Raw URL")
			}
			text = "完整内容\r\n"
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(text))}, nil
	})
	g, err := c.Get(context.Background(), "secret", "1")
	if err != nil || g.Files["big.go"].Content != "完整内容\r\n" {
		t.Fatal(g, err)
	}
	if err = c.resolveFiles(context.Background(), &Gist{Files: map[string]File{"bad": {Truncated: true, RawURL: "https://attacker.invalid/token"}}}); err == nil {
		t.Fatal("accepted untrusted Raw URL")
	}
}

func TestAuthenticationAndFailures(t *testing.T) {
	c := NewClient()
	c.HTTP.Transport = transportFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 401, Body: io.NopCloser(strings.NewReader(`{"message":"Bad credentials"}`))}, nil
	})
	if _, err := c.Session(context.Background(), ""); err == nil {
		t.Fatal("accepted missing token")
	}
	if _, err := c.Session(context.Background(), "invalid"); err == nil || !strings.Contains(err.Error(), "Bad credentials") {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	client := NewClient()
	if _, err := client.List(ctx, "secret"); err == nil {
		t.Fatal("ignored cancellation")
	}
}
