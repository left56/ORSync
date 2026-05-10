package outline

// DocumentsListRequest 对应 POST /api/documents.list 的 JSON 请求体。
// Outline 列表类接口普遍支持 limit、offset 及可选排序字段。
type DocumentsListRequest struct {
	Offset    int    `json:"offset,omitempty"`
	Limit     int    `json:"limit,omitempty"`
	Sort      string `json:"sort,omitempty"`      // 例如 updatedAt、createdAt、index
	Direction string `json:"direction,omitempty"` // ASC 或 DESC
}

// DocumentsListResponse 为 documents.list 的顶层 JSON。
type DocumentsListResponse struct {
	OK         bool               `json:"ok"`
	Status     int                `json:"status,omitempty"`
	Error      string             `json:"error"`
	Data       []DocumentListItem `json:"data"`
	Pagination Pagination         `json:"pagination"`
}

// DocumentListItem 为列表接口返回的单条文档摘要（全量同步分页遍历所需字段）。
type DocumentListItem struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	URL   string `json:"url"`
}

// Pagination 对应 Outline 列表响应根级的 pagination。
type Pagination struct {
	Limit    int    `json:"limit"`
	Offset   int    `json:"offset"`
	NextPath string `json:"nextPath,omitempty"`
}

// DocumentsInfoRequest 对应 POST /api/documents.info。
type DocumentsInfoRequest struct {
	ID string `json:"id"`
}

// DocumentsInfoResponse 为 documents.info 的顶层 JSON。
type DocumentsInfoResponse struct {
	OK     bool           `json:"ok"`
	Status int            `json:"status,omitempty"`
	Error  string         `json:"error"`
	Data   DocumentDetail `json:"data"`
}

// DocumentDetail 为 documents.info 返回的 data（含 Markdown 原文 text）。
type DocumentDetail struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	URL   string `json:"url"`
	Text  string `json:"text"`
}
