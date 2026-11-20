package outline

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// Client 封装 Outline HTTP API。
type Client struct {
	httpClient *http.Client
	baseURL    string
	apiKey     string
}

// NewClient 使用已配置的 http.Client 与 Outline 站点根 URL、API Key 构造客户端。
// baseURL 应为工作区根地址（例如 https://wiki.example.com），将自动拼接 /api/documents.* 。
// httpClient 不可为 nil。
func NewClient(httpClient *http.Client, baseURL, apiKey string) (*Client, error) {
	if httpClient == nil {
		return nil, fmt.Errorf("outline: http client is nil")
	}
	if strings.TrimSpace(baseURL) == "" {
		return nil, fmt.Errorf("outline: empty base url")
	}
	if strings.TrimSpace(apiKey) == "" {
		return nil, fmt.Errorf("outline: empty api key")
	}
	return &Client{
		httpClient: httpClient,
		baseURL:    strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		apiKey:     strings.TrimSpace(apiKey),
	}, nil
}

// ListDocuments 调用 POST /api/documents.list，返回当前页的文档摘要与分页信息。
// collectionID 为空表示不按文档集过滤（列出工作区内可见文档）；非空时仅列出该文档集内文档。
func (c *Client) ListDocuments(ctx context.Context, offset, limit int, collectionID string) (DocumentsListResponse, error) {
	if limit <= 0 {
		return DocumentsListResponse{}, fmt.Errorf("outline: limit must be > 0")
	}
	if offset < 0 {
		return DocumentsListResponse{}, fmt.Errorf("outline: offset must be >= 0")
	}
	reqBody := DocumentsListRequest{
		Offset:       offset,
		Limit:        limit,
		CollectionID: strings.TrimSpace(collectionID),
	}
	var out DocumentsListResponse
	if err := c.postJSON(ctx, "/api/documents.list", reqBody, &out); err != nil {
		return DocumentsListResponse{}, err
	}
	if !out.OK {
		return DocumentsListResponse{}, fmt.Errorf("outline documents.list: %s", outlineErrMsg(out.Error))
	}
	return out, nil
}

// DocumentView 为 GetDocument 返回的视图：元数据 + Markdown 正文。
type DocumentView struct {
	ID           string
	CollectionID string
	Title        string
	URL          string
	Text         string
}

// GetDocument 调用 POST /api/documents.info，返回标题、URL 与 Markdown 原文（text）。
func (c *Client) GetDocument(ctx context.Context, id string) (DocumentView, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return DocumentView{}, fmt.Errorf("outline: empty document id")
	}
	reqBody := DocumentsInfoRequest{ID: id}
	var out DocumentsInfoResponse
	if err := c.postJSON(ctx, "/api/documents.info", reqBody, &out); err != nil {
		return DocumentView{}, err
	}
	if !out.OK {
		return DocumentView{}, fmt.Errorf("outline documents.info: %s", outlineErrMsg(out.Error))
	}
	return DocumentView{
		ID:           out.Data.ID,
		CollectionID: out.Data.CollectionID,
		Title:        out.Data.Title,
		URL:          out.Data.URL,
		Text:         out.Data.Text,
	}, nil
}

func (c *Client) postJSON(ctx context.Context, path string, body any, out any) error {
	u, err := url.Parse(c.baseURL)
	if err != nil {
		return fmt.Errorf("outline: parse base url: %w", err)
	}
	ref, err := url.Parse(path)
	if err != nil {
		return fmt.Errorf("outline: parse path: %w", err)
	}
	full := u.ResolveReference(ref).String()

	payload, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("outline: marshal body: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, full, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("outline: new request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("outline: do request: %w", err)
	}
	defer resp.Body.Close()

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("outline: read body: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("outline: http %s: %s", resp.Status, truncateBody(b))
	}
	if err := json.Unmarshal(b, out); err != nil {
		return fmt.Errorf("outline: decode json: %w", err)
	}
	return nil
}

func outlineErrMsg(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "unknown error"
	}
	return s
}

func truncateBody(b []byte) string {
	const max = 512
	if len(b) <= max {
		return string(b)
	}
	return string(b[:max]) + "…"
}
