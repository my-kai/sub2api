package chatgpt2api

// ImageGenerationRequest 是当前扩展页支持的文生图请求。
//
// 字段保持与 chatgpt2api 的 OpenAI 兼容图片生成接口一致；response_format 由后端固定为 url，
// 让任务结果只保存 2api 返回的图片链接，避免把大块图片内容写入数据库。
type ImageGenerationRequest struct {
	Model   string `json:"model"`
	Prompt  string `json:"prompt"`
	N       int    `json:"n"`
	Quality string `json:"quality,omitempty"`
	Size    string `json:"size,omitempty"`
	// OutputFormat/OutputCompression 只给支持官方 GPT Image 字段的渠道使用；chatgpt2api 当前忽略。
	OutputFormat      string `json:"-"`
	OutputCompression int    `json:"-"`
	ResponseFormat    string `json:"response_format"`
}

// ImageEditRequest 是 OpenAI 兼容图片编辑接口所需的请求载荷。
//
// ImageURL 直接复用已完成任务里保存的 2api 图片链接；客户端会在请求上游前临时下载为 multipart，
// 避免 basketikun/chatgpt2api 对 image_url 固定 60 秒拉图超时先于本服务超时配置失败。
type ImageEditRequest struct {
	Model             string
	Prompt            string
	N                 int
	Quality           string
	Size              string
	OutputFormat      string
	OutputCompression int
	ResponseFormat    string
	ImageURL          string
	ImageBytes        []byte
	ImageFilename     string
	// ImageContentType 仅用于 multipart 文件头；为空时按 image/png 兜底。
	ImageContentType string
}

// ImageGenerationData 表示 chatgpt2api 返回的一张图片。
//
// url 是本服务唯一持久化的图片引用；不要把 b64_json 带回数据库。
type ImageGenerationData struct {
	URL           string `json:"url,omitempty"`
	RevisedPrompt string `json:"revised_prompt,omitempty"`
}

// ImageGenerationResponse 是图片生成接口返回给前端的安全子集。
type ImageGenerationResponse struct {
	Created int64                 `json:"created,omitempty"`
	Data    []ImageGenerationData `json:"data"`
}

// Model 表示 chatgpt2api /v1/models 返回的模型元数据。
type Model struct {
	ID      string `json:"id"`
	Object  string `json:"object,omitempty"`
	OwnedBy string `json:"owned_by,omitempty"`
}

// ModelsResponse 是模型列表接口返回给前端的安全子集。
type ModelsResponse struct {
	Data []Model `json:"data"`
}
