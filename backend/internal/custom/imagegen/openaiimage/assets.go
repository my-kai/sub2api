package openaiimage

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strings"
)

const defaultAssetContentType = "image/png"

// AssetURLBuilder 把 OpenAI 返回的 b64_json 转成前端可展示 URL。
//
// 当前任务不改路由和持久化表结构，所以默认实现生成 data URL；调用方后续可以在 custom 范围
// 注入文件/对象存储实现，保持 OpenAI client 不感知具体资产后端。
type AssetURLBuilder interface {
	URLForBase64Image(b64 string) (string, error)
}

// DataURLAssetBuilder 是 custom 范围内的最小可测资产归一化实现。
//
// 它不把原始 b64_json 字段透传给任务结果，而是封装成浏览器可直接展示的 data URL；
// 后续若接入持久资产存储，可替换为稳定 HTTP URL，同时保持渠道 client contract 不变。
type DataURLAssetBuilder struct {
	ContentType string
}

// URLForBase64Image 校验 base64 图片数据并返回可展示 data URL。
func (b DataURLAssetBuilder) URLForBase64Image(b64 string) (string, error) {
	trimmed := strings.TrimSpace(b64)
	if trimmed == "" {
		return "", fmt.Errorf("%w: empty b64_json image", ErrBadResponse)
	}
	contentType := strings.TrimSpace(b.ContentType)
	if contentType == "" {
		contentType = defaultAssetContentType
	}
	if _, err := base64.StdEncoding.DecodeString(trimmed); err != nil {
		return "", fmt.Errorf("%w: invalid b64_json image: %v", ErrBadResponse, err)
	}
	return "data:" + contentType + ";base64," + trimmed, nil
}

// StableAssetName 为后续持久资产实现提供确定性命名辅助。
//
// 当前 DataURL 归一化不会使用该名称；保留这个小工具是为了让后续在 custom 范围内替换为
// HTTP 资产 URL 时，不需要再把哈希细节散落到渠道编排器。
func StableAssetName(prefix string, b64 string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(b64)))
	cleanPrefix := strings.Trim(strings.ToLower(strings.TrimSpace(prefix)), "-_/ ")
	if cleanPrefix == "" {
		cleanPrefix = "openai-image"
	}
	return cleanPrefix + "-" + hex.EncodeToString(sum[:8]) + ".png"
}
