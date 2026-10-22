package gist

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

const maxBody = 32 << 20

type User struct {
	Login     string `json:"login"`
	Name      string `json:"name"`
	AvatarURL string `json:"avatar_url"`
	HTMLURL   string `json:"html_url"`
}

type File struct {
	Filename  string `json:"filename"`
	Language  string `json:"language"`
	Size      int64  `json:"size"`
	Truncated bool   `json:"truncated"`
	RawURL    string `json:"raw_url"`
	Content   string `json:"content"`
}

type Gist struct {
	ID          string          `json:"id"`
	Description string          `json:"description"`
	Public      bool            `json:"public"`
	HTMLURL     string          `json:"html_url"`
	CreatedAt   time.Time       `json:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
	Files       map[string]File `json:"files"`
	Owner       User            `json:"owner"`
}

func (g Gist) Title() string {
	if strings.TrimSpace(g.Description) != "" {
		return g.Description
	}
	names := g.Filenames()
	if len(names) > 0 {
		return names[0]
	}
	return "无描述的 Gist"
}

func (g Gist) Filenames() []string {
	names := make([]string, 0, len(g.Files))
	for name := range g.Files {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

type FileChange struct {
	Filename string  `json:"filename,omitempty"`
	Content  *string `json:"content,omitempty"`
}

type Patch struct {
	Description *string                `json:"description,omitempty"`
	Public      bool                   `json:"public,omitempty"`
	Files       map[string]*FileChange `json:"files"`
}

type APIError struct {
	Status  int
	Message string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("GitHub (%d): %s", e.Status, e.Message)
}

type Client struct {
	BaseURL string
	HTTP    *http.Client
}

func NewClient() *Client {
	return &Client{BaseURL: "https://api.github.com", HTTP: &http.Client{Timeout: 45 * time.Second}}
}

func (c *Client) request(ctx context.Context, token, method, path string, body, result any) error {
	if strings.TrimSpace(token) == "" {
		return errors.New("请输入 GitHub Token")
	}
	var input io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return err
		}
		input = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.BaseURL+path, input)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(token))
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("User-Agent", "GistManager-Native")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("连接 GitHub 失败: %w", err)
	}
	defer resp.Body.Close()
	data, err := readLimited(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		message := struct {
			Message string `json:"message"`
		}{}
		_ = json.Unmarshal(data, &message)
		if message.Message == "" {
			message.Message = http.StatusText(resp.StatusCode)
		}
		return &APIError{Status: resp.StatusCode, Message: message.Message}
	}
	if result == nil || resp.StatusCode == http.StatusNoContent {
		return nil
	}
	return json.Unmarshal(data, result)
}

func readLimited(r io.Reader) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(r, maxBody+1))
	if err == nil && len(data) > maxBody {
		err = errors.New("文件内容超过 32 MiB，请通过 GitHub 网页处理")
	}
	return data, err
}

func (c *Client) Session(ctx context.Context, token string) (User, error) {
	var user User
	err := c.request(ctx, token, http.MethodGet, "/user", nil, &user)
	return user, err
}

func (c *Client) List(ctx context.Context, token string) ([]Gist, error) {
	all := []Gist{}
	for page := 1; ; page++ {
		var items []Gist
		if err := c.request(ctx, token, http.MethodGet, fmt.Sprintf("/gists?per_page=100&page=%d", page), nil, &items); err != nil {
			return nil, err
		}
		all = append(all, items...)
		if len(items) < 100 {
			return all, nil
		}
	}
}

func (c *Client) Get(ctx context.Context, token, id string) (Gist, error) {
	var g Gist
	err := c.request(ctx, token, http.MethodGet, "/gists/"+url.PathEscape(id), nil, &g)
	if err == nil {
		err = c.resolveFiles(ctx, &g)
	}
	return g, err
}

func (c *Client) Create(ctx context.Context, token string, patch Patch) (Gist, error) {
	return c.mutate(ctx, token, http.MethodPost, "/gists", patch)
}

func (c *Client) Update(ctx context.Context, token, id string, patch Patch) (Gist, error) {
	return c.mutate(ctx, token, http.MethodPatch, "/gists/"+url.PathEscape(id), patch)
}

func (c *Client) mutate(ctx context.Context, token, method, path string, patch Patch) (Gist, error) {
	var g Gist
	if len(patch.Files) == 0 {
		return g, errors.New("至少需要一个文件变更")
	}
	err := c.request(ctx, token, method, path, patch, &g)
	if err == nil {
		err = c.resolveFiles(ctx, &g)
	}
	return g, err
}

func (c *Client) Delete(ctx context.Context, token, id string) error {
	return c.request(ctx, token, http.MethodDelete, "/gists/"+url.PathEscape(id), nil, nil)
}

func (c *Client) resolveFiles(ctx context.Context, g *Gist) error {
	for name, file := range g.Files {
		if !file.Truncated {
			continue
		}
		u, err := url.Parse(file.RawURL)
		if err != nil || u.Scheme != "https" || (u.Hostname() != "gist.githubusercontent.com" && u.Hostname() != "raw.githubusercontent.com") {
			return fmt.Errorf("文件 %s 的 Raw 地址不可信", name)
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, file.RawURL, nil)
		if err != nil {
			return err
		}
		resp, err := c.HTTP.Do(req)
		if err != nil {
			return err
		}
		data, readErr := readLimited(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("读取 %s 失败 (%d)", name, resp.StatusCode)
		}
		if readErr != nil {
			return readErr
		}
		file.Content, file.Truncated = string(data), false
		g.Files[name] = file
	}
	return nil
}
