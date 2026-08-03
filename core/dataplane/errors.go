package dataplane

import (
	"fmt"
	"strings"
)

// UserFacingError maps internal ops/xDS errors to short, actionable Chinese for Panel/CLI.
// Technical detail should stay in logs or the "output" field, not as the sole toast.
func UserFacingError(err error) string {
	if err == nil {
		return ""
	}
	msg := err.Error()
	low := strings.ToLower(msg)

	switch {
	case strings.Contains(low, "ack timeout"), strings.Contains(msg, "ACK timeout"):
		if strings.Contains(low, "rollback") || strings.Contains(msg, "回滚") {
			return "Envoy 未在时限内确认新配置，且回滚上一快照失败。请立即执行硬重启（reload --hard），并检查本机 Panel 与 Envoy 是否正常。"
		}
		return "Envoy 未在时限内确认新配置，已尝试回滚上一快照。请检查本机 Panel 与 Envoy；仍异常时执行 reload --hard。"
	case strings.Contains(low, "healthcheck/fail"):
		return "摘流失败：无法接通本机 Envoy 探活接口。请确认 Envoy 已运行后重试。"
	case strings.Contains(low, "healthcheck/ok"):
		return "恢复承接失败：无法接通本机 Envoy 探活接口。请确认 Envoy 已运行后重试。"
	case strings.Contains(low, "admin unreachable"), strings.Contains(msg, "admin 不可达"):
		return "无法连接 Envoy 管理口，热更新无法确认是否生效。请确认 Envoy 已运行，或改用硬重启应用。"
	case strings.Contains(low, "update_rejected"), strings.Contains(msg, "配置被拒绝"):
		return "Envoy 拒绝了新配置（已尝试回滚）。请检查资源校验与 Envoy 日志，必要时执行 reload --hard。"
	case strings.Contains(low, "no previous snapshot"), strings.Contains(msg, "无上一版快照"):
		return "无法回滚上一快照（尚无历史版本）。请执行 reload --hard 恢复，并确认 Panel 已正常启动。"
	case strings.Contains(low, "mgmt") && strings.Contains(low, "not listening"):
		return "本机热更新服务未就绪。请启动 Panel（主控）或 agent run（节点）。"
	case strings.Contains(msg, "xds ADS"), strings.Contains(low, "xds:"):
		return "本机热更新服务不可用。请确认 Panel 或 agent run 已运行，或执行 reload --hard。"
	case strings.Contains(msg, "bootstrap"), strings.Contains(msg, "未迁移"), strings.Contains(msg, "dynamic_resources"):
		return "本机尚未启用热更新。请执行一次 reload --hard，之后即可热更新。"
	case strings.Contains(msg, "docker 不可用"), strings.Contains(low, "docker"):
		if strings.Contains(msg, "validate") || strings.Contains(msg, "校验") {
			return "无法校验 Envoy 配置（docker 不可用或校验容器失败）。热更新已中止，请修复 docker 后重试。"
		}
	case strings.Contains(msg, "standby"):
		return "当前节点为只读，禁止写操作。请到主控执行。"
	case strings.Contains(low, "exit status"):
		return "操作未成功完成。请查看下方输出或运维日志中的详细信息后重试。"
	}
	return msg
}

// WrapUserFacing returns an error whose Error() is user-facing, preserving the cause via Unwrap.
func WrapUserFacing(err error) error {
	if err == nil {
		return nil
	}
	human := UserFacingError(err)
	if human == err.Error() {
		return err
	}
	return &userFacingError{human: human, cause: err}
}

type userFacingError struct {
	human string
	cause error
}

func (e *userFacingError) Error() string { return e.human }
func (e *userFacingError) Unwrap() error { return e.cause }
func (e *userFacingError) Format(f fmt.State, verb rune) {
	if verb == 'v' && f.Flag('+') {
		fmt.Fprintf(f, "%s\n%+v", e.human, e.cause)
		return
	}
	fmt.Fprint(f, e.human)
}
