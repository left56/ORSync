package ragflow

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"strings"
	"unicode/utf8"
)

// Client 封装 RAGFlow 数据集文档相关 HTTP API。
// 上传与删除路径遵循当前 RAGFlow HTTP 规范：/api/v1/datasets/{dataset_id}/documents （datasets、documents 均为复数）。
type Client struct {
	httpClient *http.Client
	baseURL    string
	apiKey     string
	datasetID  string
}

// NewClient 使用已配置的 http.Client、RAGFlow 根 URL、API Key 与数据集 ID 构造客户端。
// baseURL 为服务根地址（例如 http://127.0.0.1:9380），不含具体 API path。
// httpClient 不可为 nil。
func NewClient(httpClient *http.Client, baseURL, apiKey, datasetID string) (*Client, error) {
	if httpClient == nil {
		return nil, fmt.Errorf("ragflow: http client is nil")
	}
	if strings.TrimSpace(baseURL) == "" {
		return nil, fmt.Errorf("ragflow: empty base url")
	}
	if strings.TrimSpace(apiKey) == "" {
		return nil, fmt.Errorf("ragflow: empty api key")
	}
	if strings.TrimSpace(datasetID) == "" {
		return nil, fmt.Errorf("ragflow: empty dataset id")
	}
	return &Client{
		httpClient: httpClient,
		baseURL:    strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		apiKey:     strings.TrimSpace(apiKey),
		datasetID:  strings.TrimSpace(datasetID),
	}, nil
}

// UploadDocument 使用 multipart/form-data，字段名为 file，以内存中的 Markdown 内容上传。
// 文件名使用 .md 后缀以触发 Markdown 解析；为兼顾文件名合法性与可读性，优先基于 docID 命名。
func (c *Client) UploadDocument(ctx context.Context, docID, title string, markdownContent []byte) (UploadedDoc, error) {
	docID = strings.TrimSpace(docID)
	if docID == "" {
		return UploadedDoc{}, fmt.Errorf("ragflow: empty doc id")
	}
	if markdownContent == nil {
		return UploadedDoc{}, fmt.Errorf("ragflow: markdown content is nil")
	}

	filename := uploadFilename(docID, title)
	path := fmt.Sprintf("/api/v1/datasets/%s/documents", url.PathEscape(c.datasetID))
	full, err := c.joinPath(path)
	if err != nil {
		return UploadedDoc{}, err
	}

	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	part, err := w.CreateFormFile("file", filename)
	if err != nil {
		return UploadedDoc{}, fmt.Errorf("ragflow: create form file: %w", err)
	}
	if _, err := part.Write(markdownContent); err != nil {
		return UploadedDoc{}, fmt.Errorf("ragflow: write file part: %w", err)
	}
	if err := w.Close(); err != nil {
		return UploadedDoc{}, fmt.Errorf("ragflow: close multipart: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, full, &body)
	if err != nil {
		return UploadedDoc{}, fmt.Errorf("ragflow: new request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", w.FormDataContentType())

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return UploadedDoc{}, fmt.Errorf("ragflow: do upload: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return UploadedDoc{}, fmt.Errorf("ragflow: read upload body: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return UploadedDoc{}, fmt.Errorf("ragflow: upload http %s: %s", resp.Status, truncateBody(raw))
	}

	var out UploadDocumentResponse
	if err := json.Unmarshal(raw, &out); err != nil {
		return UploadedDoc{}, fmt.Errorf("ragflow: decode upload json: %w", err)
	}
	if out.Code != 0 {
		return UploadedDoc{}, fmt.Errorf("ragflow: upload code=%d: %s", out.Code, ragflowErrMsg(out.Message))
	}
	if len(out.Data) == 0 {
		return UploadedDoc{}, fmt.Errorf("ragflow: upload succeeded but empty data")
	}
	return out.Data[0], nil
}

// DeleteDocument 调用 DELETE /api/v1/datasets/{dataset_id}/documents，在 JSON Body 中传入文档 ID 列表。
func (c *Client) DeleteDocument(ctx context.Context, documentIDs []string) error {
	if len(documentIDs) == 0 {
		return fmt.Errorf("ragflow: no document ids to delete")
	}
	path := fmt.Sprintf("/api/v1/datasets/%s/documents", url.PathEscape(c.datasetID))
	full, err := c.joinPath(path)
	if err != nil {
		return err
	}

	payload := DeleteDocumentsRequest{IDs: documentIDs}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("ragflow: marshal delete body: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, full, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("ragflow: new delete request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("ragflow: do delete: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("ragflow: read delete body: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("ragflow: delete http %s: %s", resp.Status, truncateBody(raw))
	}

	var out DeleteDocumentsResponse
	if err := json.Unmarshal(raw, &out); err != nil {
		return fmt.Errorf("ragflow: decode delete json: %w", err)
	}
	if out.Code != 0 {
		return fmt.Errorf("ragflow: delete code=%d: %s", out.Code, ragflowErrMsg(out.Message))
	}
	return nil
}

func (c *Client) joinPath(path string) (string, error) {
	u, err := url.Parse(c.baseURL)
	if err != nil {
		return "", fmt.Errorf("ragflow: parse base url: %w", err)
	}
	ref, err := url.Parse(path)
	if err != nil {
		return "", fmt.Errorf("ragflow: parse path: %w", err)
	}
	return u.ResolveReference(ref).String(), nil
}

func uploadFilename(docID, title string) string {
	const maxRunes = 100
	base := strings.TrimSpace(title)
	base = strings.Map(func(r rune) rune {
		switch r {
		case '/', '\\', '<', '>', ':', '"', '|', '?', '*':
			return '_'
		default:
			return r
		}
	}, base)
	base = strings.TrimSpace(base)
	if base == "" {
		base = docID
	}
	if utf8.RuneCountInString(base) > maxRunes {
		r := []rune(base)
		base = string(r[:maxRunes])
	}
	lower := strings.ToLower(base)
	if strings.HasSuffix(lower, ".md") {
		return base
	}
	return base + ".md"
}

func ragflowErrMsg(s string) string {
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
