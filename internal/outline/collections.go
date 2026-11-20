package outline

import (
	"context"
	"fmt"
	"sort"
	"strings"
)

const defaultCollectionsPageSize = 100

// CollectionsListRequest 对应 POST /api/collections.list。
type CollectionsListRequest struct {
	Offset int    `json:"offset,omitempty"`
	Limit  int    `json:"limit,omitempty"`
	Sort   string `json:"sort,omitempty"`
	// Query 若设置则按文档集名称筛选（Outline 服务端过滤）。
	Query string `json:"query,omitempty"`
}

// CollectionsListResponse 为 collections.list 的顶层 JSON。
type CollectionsListResponse struct {
	OK         bool                `json:"ok"`
	Status     int                 `json:"status,omitempty"`
	Error      string              `json:"error"`
	Data       []CollectionSummary `json:"data"`
	Pagination Pagination          `json:"pagination"`
}

// CollectionSummary 为列表接口返回的文档集摘要。
type CollectionSummary struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// ListCollections 调用 POST /api/collections.list，返回当前页的文档集与分页信息。
func (c *Client) ListCollections(ctx context.Context, offset, limit int) (CollectionsListResponse, error) {
	if limit <= 0 {
		return CollectionsListResponse{}, fmt.Errorf("outline: collections list limit must be > 0")
	}
	if offset < 0 {
		return CollectionsListResponse{}, fmt.Errorf("outline: collections list offset must be >= 0")
	}
	reqBody := CollectionsListRequest{
		Offset: offset,
		Limit:  limit,
	}
	var out CollectionsListResponse
	if err := c.postJSON(ctx, "/api/collections.list", reqBody, &out); err != nil {
		return CollectionsListResponse{}, err
	}
	if !out.OK {
		return CollectionsListResponse{}, fmt.Errorf("outline collections.list: %s", outlineErrMsg(out.Error))
	}
	return out, nil
}

// ListAllCollections 分页拉取当前 API Key 可见的全部文档集。
func (c *Client) ListAllCollections(ctx context.Context) ([]CollectionSummary, error) {
	var all []CollectionSummary
	offset := 0
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		page, err := c.ListCollections(ctx, offset, defaultCollectionsPageSize)
		if err != nil {
			return nil, err
		}
		if len(page.Data) == 0 {
			break
		}
		all = append(all, page.Data...)
		offset += len(page.Data)
		if len(page.Data) < defaultCollectionsPageSize {
			break
		}
	}
	return all, nil
}

// ResolveCollectionNamesToIDs 将若干文档集名称（不区分大小写、去首尾空白）解析为 Outline 的 collection UUID。
// 任一名称在可见文档集中找不到匹配时返回错误；同名多文档集会全部加入结果并去重。
func ResolveCollectionNamesToIDs(ctx context.Context, c *Client, names []string) ([]string, error) {
	if c == nil {
		return nil, fmt.Errorf("outline: client is nil")
	}
	wanted := NormalizeCollectionNameList(names)
	if len(wanted) == 0 {
		return nil, fmt.Errorf("outline: no non-empty collection names to resolve")
	}
	colls, err := c.ListAllCollections(ctx)
	if err != nil {
		return nil, err
	}
	idSet := make(map[string]struct{})
	matched := make(map[string]struct{}, len(wanted))
	for _, want := range wanted {
		for _, col := range colls {
			if strings.EqualFold(strings.TrimSpace(col.Name), want) {
				idSet[col.ID] = struct{}{}
				matched[want] = struct{}{}
			}
		}
	}
	var missing []string
	for _, want := range wanted {
		if _, ok := matched[want]; !ok {
			missing = append(missing, want)
		}
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("outline: unknown collection name(s): %s", strings.Join(missing, ", "))
	}
	out := make([]string, 0, len(idSet))
	for id := range idSet {
		out = append(out, id)
	}
	sort.Strings(out)
	return out, nil
}

// NormalizeCollectionNameList 去空项、首尾空白，并按不区分大小写去重（保留每种大小写变体的首次出现）。
func NormalizeCollectionNameList(names []string) []string {
	seen := make(map[string]struct{})
	var out []string
	for _, raw := range names {
		n := strings.TrimSpace(raw)
		if n == "" {
			continue
		}
		key := strings.ToLower(n)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, n)
	}
	return out
}
