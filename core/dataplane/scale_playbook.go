package dataplane

import (
	"fmt"
	"strings"
)

// ScalePlaybookStep is one item in join/leave preview.
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
	switch mode {
	case "join":
		return ScalePlaybook{
			Mode: "join",
			Steps: []ScalePlaybookStep{
				{
					ID:          "token",
					Title:       "生成接入命令",
					Description: "主控写入节点名册、签发一次性令牌，并给出一句话安装命令。",
					Checklist:   []string{"已选定节点名称"},
					Commands:    []string{"relaygate fleet join <name>  # 生成一句话安装命令", "POST /api/ops/fleet/join"},
					Automated:   true,
				},
				{
					ID:          "install-node",
					Title:       "目标机执行一句话安装",
					Description: "在目标主机以 root 粘贴 join 输出的一行命令：下载安装、写令牌、启动 agent。",
					Checklist:   []string{"已执行一句话命令", "relaygate-agent 已启动并出现在机群列表"},
					Commands:    []string{"# 使用 fleet join 输出的 curl | sudo env … bash -s -- -y"},
					Automated:   false,
					DocHref:     "packaging/node/env.example",
				},
				{
					ID:          "nlb-register",
					Title:       "可选云入口挂载",
					Description: "仅当前置了云 L4 入口时：在云控制台或 Terraform 将新节点注册到目标组（本产品不调用云 API）。公网直连网关可跳过。",
					Checklist:   []string{"若无云入口可跳过", "有云入口时：目标组 register 完成", "健康检查通过"},
					Commands:    []string{"# 可选 Terraform: packaging/terraform/nlb/"},
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
				},
				{
					ID:          "nlb-deregister",
					Title:       "可选云入口摘除",
					Description: "仅当前置了云 L4 入口时：在云控制台/TF 从目标组摘除。本流程不调用云 API；公网直连可跳过。",
					Checklist:   []string{"若无云入口可跳过", "有云入口时：目标组 deregister 完成"},
					Commands:    []string{"# 可选：云控制台 / Terraform deregister"},
					Manual:      true,
					DocHref:     "packaging/terraform/nlb/",
				},
			},
		}, nil
	default:
		return ScalePlaybook{}, fmt.Errorf("mode 须为 join 或 leave")
	}
}
