package dashboard

import (
	mcpserver "websearch/mcp"
	"websearch/pkg/config"
)

// MCPToolView 是控制中心展示的公开 MCP 工具清单条目。它与
// telemetry.Overview.Tools 不同：后者只包含产生过真实调用的事件聚合。
type MCPToolView struct {
	Name        string        `json:"name"`
	Label       string        `json:"label"`
	Enabled     bool          `json:"enabled"`
	Registered  bool          `json:"registered"`
	State       string        `json:"state"`
	Description string        `json:"description"`
	PDF         *PDFToolView  `json:"pdf,omitempty"`
	File        *FileToolView `json:"file,omitempty"`
}

type FileToolView struct {
	AllowedRootCount  int  `json:"allowed_root_count"`
	PathMetadataOnly  bool `json:"path_metadata_only"`
	WhitelistEnforced bool `json:"whitelist_enforced"`
}

type PDFToolView struct {
	MaxPages            int    `json:"max_pages"`
	MinerUPageLimit     int    `json:"mineru_page_limit"`
	MinerUPageBatchSize int    `json:"mineru_page_batch_size"`
	MinerUPageBudget    int    `json:"mineru_page_budget"`
	OCREnabled          bool   `json:"ocr_enabled"`
	PageProgress        string `json:"page_progress"`
}

// mcpToolCatalog 定义控制中心固定展示的公开 MCP 工具。Enabled 只反映当前
// 配置开关，不表示已经产生过真实调用；观测状态由遥测事件补充。
func mcpToolCatalog(conf config.Config) []MCPToolView {
	tools := []MCPToolView{
		{
			Name:        "smartsearch",
			Label:       "智能搜索",
			Enabled:     len(activeSourceIDs(webSourceCatalog(conf))) > 0,
			Description: "通用联网检索工具",
		},
		{
			Name:        "academicsearch",
			Label:       "学术检索",
			Enabled:     conf.Academic.Enabled,
			Description: "多学术数据库并行检索",
		},
		{
			Name:        "cleanfetch",
			Label:       "网页抓取",
			Enabled:     conf.CleanFetch.Enabled,
			Description: "网页正文抓取与 Markdown 提取",
		},
		{
			Name:        "pdf_parser",
			Label:       "PDF 解析",
			Enabled:     conf.PDFParser.Enabled,
			Description: "PDF 文本提取与 MinerU 回退",
			PDF: &PDFToolView{MaxPages: conf.PDFParser.GetMaxPages(),
				MinerUPageLimit:     conf.PDFParser.GetMinerUPageLimit(),
				MinerUPageBatchSize: conf.PDFParser.GetMinerUPageBatchSize(),
				MinerUPageBudget:    conf.PDFParser.GetMinerUPageBudget(),
				OCREnabled:          conf.PDFParser.MinerUOCREnabled(), PageProgress: "not_collected"},
		},
		{
			Name: "file_search", Label: "本地文件检索", Enabled: conf.Everything.URL != "",
			Description: "Everything 本地文件索引检索（只读）",
			File: &FileToolView{AllowedRootCount: len(conf.Everything.Roots),
				PathMetadataOnly: true, WhitelistEnforced: len(conf.Everything.Roots) > 0},
		},
	}
	states := mcpserver.ToolSnapshot()
	for i := range tools {
		for _, state := range states {
			if tools[i].Name == state.Name {
				tools[i].State, tools[i].Registered = state.State, state.Registered
			}
		}
	}
	return tools
}
