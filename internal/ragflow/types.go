package ragflow

// UploadDocumentResponse 为上传文档接口的 JSON 顶层结构（RAGFlow 约定 code==0 表示成功）。
type UploadDocumentResponse struct {
	Code    int           `json:"code"`
	Message string        `json:"message"`
	Data    []UploadedDoc `json:"data"`
}

// UploadedDoc 表示上传成功后的单条文档元信息。
type UploadedDoc struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	DatasetID  string `json:"dataset_id"`
	Size       int64  `json:"size"`
	CreateTime int64  `json:"create_time"`
}

// DeleteDocumentsRequest 为 DELETE /api/v1/datasets/{dataset_id}/documents 的 JSON Body。
// 官方实现使用字段名 ids；误用 document_ids 可能导致未按预期删除（参见 RAGFlow 历史 issue）。
type DeleteDocumentsRequest struct {
	IDs       []string `json:"ids"`
	DeleteAll bool     `json:"delete_all,omitempty"`
}

// DeleteDocumentsResponse 为删除接口的 JSON 顶层结构。
type DeleteDocumentsResponse struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}
