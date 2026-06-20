package handler

import (
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/custom/imagegen/imagequeue"
)

const publicImageTaskFailureMessage = "生图任务执行失败，请稍后重试"

// sanitizePublicImageJobs 在所有用户可见任务出口前清洗历史失败原因。
//
// 新 worker 已经会写入通用失败文案，但数据库里可能存在旧版本保存的 `chatgpt2api`、渠道名称或上游
// 原始错误；handler 作为返回前端前的最后一道边界，需要兜底处理这些历史值。
func sanitizePublicImageJobs(jobs []imagequeue.Job) {
	for index := range jobs {
		sanitizePublicImageJob(&jobs[index])
	}
}

// sanitizePublicImageJob 只改响应副本，不回写数据库，避免列表读取顺手改变任务历史。
func sanitizePublicImageJob(job *imagequeue.Job) {
	if job == nil || job.Status != imagequeue.JobStatusFailed {
		return
	}
	if leaksUpstreamImageDetail(job.ErrorMessage) {
		job.ErrorMessage = publicImageTaskFailureMessage
	}
}

// leaksUpstreamImageDetail 识别不应出现在前端的上游渠道和供应商诊断词。
//
// 这里故意只做保守关键字过滤：普通业务校验错误仍按原文展示；渠道、鉴权和上游故障类信息统一收口。
func leaksUpstreamImageDetail(message string) bool {
	lower := strings.ToLower(strings.TrimSpace(message))
	if lower == "" {
		return false
	}
	for _, marker := range []string{
		"chatgpt2api",
		"openai image",
		"upstream",
		"auth key",
		"authorization",
		"bearer",
		"all image upstream channels failed",
	} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}
