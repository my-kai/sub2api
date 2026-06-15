package openaiimage

// GenerateRequest 是 OpenAI 官方图片生成接口支持的请求子集。
//
// 调用方传入的字段来自 custom 队列已校验后的任务；客户端只发送官方图片接口可识别字段，
// 避免把 chatgpt2api 的 response_format=url 兼容参数泄露到官方渠道。
type GenerateRequest struct {
	Model             string `json:"model"`
	Prompt            string `json:"prompt"`
	N                 int    `json:"n,omitempty"`
	Quality           string `json:"quality,omitempty"`
	Size              string `json:"size,omitempty"`
	OutputFormat      string `json:"output_format,omitempty"`
	OutputCompression int    `json:"output_compression,omitempty"`
}

// EditRequest 是 OpenAI 官方图片编辑接口支持的请求子集。
//
// ImageBytes 必须由调用方在进入渠道重试前准备好；来源图下载失败不属于上游渠道失败，
// 因此不应隐藏在 client 内部重试。
type EditRequest struct {
	Model             string
	Prompt            string
	N                 int
	Quality           string
	Size              string
	OutputFormat      string
	OutputCompression int
	ImageBytes        []byte
	ImageFilename     string
	ImageContentType  string
}

// ResponseData 是 OpenAI 图片响应中的单张图片结果。
type ResponseData struct {
	URL           string `json:"url,omitempty"`
	B64JSON       string `json:"b64_json,omitempty"`
	RevisedPrompt string `json:"revised_prompt,omitempty"`
}

// Response 是 OpenAI 图片接口返回体的安全子集。
type Response struct {
	Created int64          `json:"created,omitempty"`
	Data    []ResponseData `json:"data"`
}
