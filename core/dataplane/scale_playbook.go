package dataplane

import (
	"fmt"
	"strings"
)

// ScalePlaybookStep is one item in join/leave preview (legacy type name retained in ops package).
type ScalePlaybookStep struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Checklist   []string `json:"checklist"`
	Commands    []string `json:"commands"`
	Manual      bool     `json:"manual"`
	Automated   bool     `json:"automated,omitempty"`
	DocHref     string   `json:"doc_href,omitempty"`
}

// ScalePlaybook is a join/leave checklist preview.
type ScalePlaybook struct {
	Mode  string              `json:"mode"`
	Steps []ScalePlaybookStep `json:"steps"`
}

// BuildScalePlaybook returns join/leave checklist (mode: join|leave；兼容旧 expand|shrink 别名已废除，仅 join/leave).
func BuildScalePlaybook(root, mode string) (ScalePlaybook, error) {
	_ = root
	mode = strings.ToLower(strings.TrimSpace(mode))
	doc := "docs/product-surface-agent.md"
	switch mode {
	case "join":
		return ScalePlaybook{
			Mode: "join",
			Steps: []ScalePlaybookStep{
				{
					ID:          "token",
					Title:       "生成接入令牌",
					Description: "主控写入节点名册并签发一次性代理令牌。",
					Checklist:   []string{"已选定节点名称"},
					Commands:    []string{"relaygate fleet join <name>  # 确认 FLEET_JOIN", "POST /api/ops/fleet/join"},
					Automated:   true,
					DocHref:     doc,
				},
				{
					ID:          "install-node",
					Title:       "目标机安装节点组件",
					Description: "使用 node.env.example：ENABLE_PANEL=0、PRIMARY_URL、AGENT_TOKEN_FILE。",
					Checklist:   []string{"令牌已写入 AGENT_TOKEN_FILE", "relaygate agent run 已启动"},
					Commands:    []string{"cp packaging/node/env.example .env", "relaygate agent run"},
					Automated:   false,
					DocHref:     "packaging/node/env.example",
				},
				{
					ID:          "nlb-register",
					Title:       "负载均衡挂载",
					Description: "云控制台或 Terraform 将新节点注册到目标组（不接云 API）。",
					Checklist:   []string{"目标组 register 完成", "健康检查通过"},
					Commands:    []string{"# Terraform: packaging/terraform/nlb/"},
					Manual:      true,
					DocHref:     "packaging/terraform/nlb/",
				},
			},
		}, nil
	case "leave":
		return ScalePlaybook{
			Mode: "leave",
			Steps: []ScalePlaybookStep{
				{
					ID:          "leave-registry",
					Title:       "名册移除与吊销令牌",
					Description: "从节点名册移除并吊销代理凭证。",
					Checklist:   []string{"已确认退役窗口"},
					Commands:    []string{"relaygate fleet leave <name>  # 确认 FLEET_LEAVE", "POST /api/ops/fleet/leave"},
					Automated:   true,
					DocHref:     doc,
				},
				{
					ID:          "nlb-deregister",
					Title:       "负载均衡摘除",
					Description: "云控制台/TF 从目标组摘除。本流程不调用云 API。",
					Checklist:   []string{"目标组 deregister 完成"},
					Commands:    []string{"# 云控制台 / Terraform deregister"},
					Manual:      true,
					DocHref:     "packaging/terraform/nlb/",
				},
			},
		}, nil
	default:
		return ScalePlaybook{}, fmt.Errorf("mode 须为 join 或 leave")
	}
}
