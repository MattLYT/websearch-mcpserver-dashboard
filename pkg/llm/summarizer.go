package llm

import (
	"context"
	"fmt"
	"websearch/pkg/log"
	"websearch/pkg/search/core"
)

type Summarizer struct {
	llm *Client
}

func NewSummarizer(baseURL, apiKey, model_id string) *Summarizer {
	return &Summarizer{
		llm: NewClient(baseURL, apiKey, model_id),
	}
}

// Summarize 搜索完成后，根据 query + intent + 搜索结果调用 LLM 生成摘要
func (s *Summarizer) Summarize(query, intent string, results []core.SearchResult) (string, error) {
	userPrompt := BuildUserPrompt(query, intent, results)
	log.Infof("调用 LLM 生成摘要, query=%s, intent=%s", query, intent)

	summary, err := s.llm.Chat(systemPrompt, userPrompt)
	if err != nil {
		return "", fmt.Errorf("LLM 摘要生成失败: %w", err)
	}

	output := FormatCitation(query, summary, results)
	return output, nil
}

// SummarizeStream 流式生成摘要，通过 ch 逐 token 推送原始 LLM 输出。
// 流结束时关闭 ch，并在 errCh 发送一次结果（nil 表示成功）。
// 调用方负责消费 ch 并累积全文；ctx 取消（如客户端断开）时自动中止。
// 完整摘要的引用格式化由调用方在流结束后通过 FormatCitation 完成。
func (s *Summarizer) SummarizeStream(ctx context.Context, query, intent string, results []core.SearchResult, ch chan<- string, errCh chan<- error) {
	userPrompt := BuildUserPrompt(query, intent, results)
	log.Infof("调用 LLM 流式生成摘要, query=%s, intent=%s", query, intent)
	s.llm.ChatStream(ctx, systemPrompt, userPrompt, ch, errCh)
}
