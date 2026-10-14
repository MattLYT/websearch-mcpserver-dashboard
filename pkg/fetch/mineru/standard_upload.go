package mineru

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
)

// ParseStandardFile sends a locally cropped PDF through MinerU's signed upload API.
func (c *Client) ParseStandardFile(ctx context.Context, filePath string) (string, error) {
	if c.token == "" {
		return "", fmt.Errorf("MinerU 精准解析 API 需要 Token")
	}
	info, err := os.Stat(filePath)
	if err != nil {
		return "", err
	}
	if info.Size() > 200*1024*1024 {
		return "", fmt.Errorf("裁切后的 PDF 仍超过 MinerU 精准 API 的 200MB 限制")
	}
	batchID, uploadURL, err := c.createStandardUpload(ctx, filepath.Base(filePath))
	if err != nil {
		return "", err
	}
	if err := c.uploadFile(ctx, uploadURL, filePath); err != nil {
		return "", fmt.Errorf("MinerU 文件上传失败: %w；task_ref=%q", err, "batch:"+batchID)
	}
	zipURL, err := c.pollStandardUpload(ctx, batchID)
	if err != nil {
		return "", err
	}
	md, err := c.downloadZIP(ctx, zipURL)
	if err != nil {
		return "", fmt.Errorf("MinerU 结果下载失败: %w；task_ref=%q", err, "batch:"+batchID)
	}
	return fmt.Sprintf("> **MinerU 完整结果 ZIP（含图片资源）**：%s。Markdown 中的相对图片路径需与 ZIP 内的 images/ 一起使用。\n\n%s", zipURL, md), nil
}

type standardUploadResponse struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
	Data struct {
		BatchID  string   `json:"batch_id"`
		FileURLs []string `json:"file_urls"`
	} `json:"data"`
}

func (c *Client) createStandardUpload(ctx context.Context, name string) (string, string, error) {
	body := map[string]any{
		"files":          []map[string]any{{"name": name, "is_ocr": c.ocr}},
		"model_version":  c.modelVersion,
		"enable_formula": c.formula,
		"enable_table":   c.table,
		"language":       c.lang,
	}
	var result standardUploadResponse
	resp, err := c.client.R().SetContext(ctx).
		SetHeader("Content-Type", "application/json").
		SetHeader("Authorization", "Bearer "+c.token).
		SetBody(body).SetResult(&result).
		Post(c.endpoint + standardAPIPath + "/file-urls/batch")
	if err != nil {
		return "", "", fmt.Errorf("MinerU 服务连接失败: %w", err)
	}
	if resp.StatusCode() != http.StatusOK {
		return "", "", fmt.Errorf("MinerU 服务异常 (HTTP %d)", resp.StatusCode())
	}
	if result.Code != 0 {
		return "", "", fmt.Errorf("MinerU 上传任务创建失败: %s", mapAPIError(result.Code, result.Msg))
	}
	if result.Data.BatchID == "" || len(result.Data.FileURLs) != 1 || result.Data.FileURLs[0] == "" {
		return "", "", fmt.Errorf("MinerU 未返回有效的文件上传链接")
	}
	return result.Data.BatchID, result.Data.FileURLs[0], nil
}

func (c *Client) pollStandardUpload(ctx context.Context, batchID string) (string, error) {
	return c.pollTask(ctx, "batch:"+batchID)
}
