package service

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// ISS-R4-0045：脱敏和拒单原本共用一条正则，于是两头都不对——宽的那一头漏掉
// AWS/GitHub/Slack/JWT，窄的那一头把文档里的 api_key:/password: 当成凭据拒单。
// 下面两组各自只断言自己那个目标。

// 工单/公告里最常被粘的凭据形态，八类都得整段抹掉。
func TestAdtyRedactPatternCatchesPastedCredentials(t *testing.T) {
	cases := []struct {
		name   string
		text   string
		secret string
	}{
		{"OpenAI key", "Authorization: Bearer sk-proj-abcdEFGH12345678", "sk-proj-abcdEFGH12345678"},
		{"Telegram bot", "token bot123456789:AAHdqTcvCH1vGWJxfSeofSAs0K5PALDsaw", "AAHdqTcvCH1vGWJxfSeofSAs0K5PALDsaw"},
		{"Bearer JWT", "Authorization: Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxIn0.abc12345678", "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9"},
		{"裸 JWT", "token=eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxMjM0In0.aBcDeFgHiJkL", "eyJhbGciOiJIUzI1NiJ9"},
		{"AWS AKIA", "aws_access_key_id = AKIAIOSFODNN7EXAMPLE", "AKIAIOSFODNN7EXAMPLE"},
		{"GitHub PAT", "ghp_aBcDeFgHiJkLmNoPqRsTuVwXyZ0123456789", "ghp_aBcDeFgHiJkLmNoPqRsTuVwXyZ0123456789"},
		{"GitHub 细粒度 PAT", "github_pat_11ABCDEFG0123456789_abcdefghijklmnop", "github_pat_11ABCDEFG0123456789_abcdefghijklmnop"},
		{"Slack", "xoxb-123456789012-abcdefghijkl", "xoxb-123456789012-abcdefghijkl"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := redactAdtySecrets(c.text)
			assert.NotContains(t, got, c.secret, "凭据原文必须整段消失")
			assert.Contains(t, got, "[redacted]", "脱敏要留可见痕迹")
		})
	}
}

// 正常文本一字不改：多打一段码只是难看，但把这些打码会让管理员看不懂工单。
func TestAdtyRedactPatternKeepsOrdinaryText(t *testing.T) {
	cases := []struct {
		name string
		text string
	}{
		{"错误日志+请求ID", `{"error":"invalid_request","request_id":"a3f8c2d1e4b5a6f7"}`},
		{"CDN 图片 URL 含 sha256", "https://cdn.example.com/img/9f86d081884c7d659a2feaa0c55ad015.png"},
		{"base64(JSON) 内联", "data:application/json;base64,eyJtb2RlbCI6ImdwdC00byIsIm4iOjF9"},
		{"base64(JSON) 另一例", "payload=eyJlcnJvciI6InJhdGVfbGltaXQifQ=="},
		{"翻译：密码策略文档", "请在设置页点击 password 后方的箭头，再选择修改。"},
		{"翻译：JWT 技术文档", "JWT 由三段组成，第一段是 eyJhbGciOiJIUzI1NiJ9 这样的头。"},
		{"公告：风控说明", "本次风控升级会影响 api_key 的轮换流程，请提前准备。"},
		{"构建号", "build=20261115-01a2b3c4  deployed at 08:31"},
		{"数值+哈希日志", "task 123456: 9f86d081884c7d659a2feaa0c55ad015 finished in 3.2s"},
		{"Git 提交说明", "fix: rotate github_pat_ secret handling"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.text, redactAdtySecrets(c.text))
		})
	}
}

// 拒单只认「几乎不可能是正常文本」的形态。
func TestAdtyRejectPatternCatchesBareCredentials(t *testing.T) {
	cases := []struct {
		name string
		text string
	}{
		{"OpenAI key", "Authorization: Bearer sk-proj-abcdEFGH12345678"},
		{"Telegram bot", "token bot123456789:AAHdqTcvCH1vGWJxfSeofSAs0K5PALDsaw"},
		{"AWS AKIA", "aws_access_key_id = AKIAIOSFODNN7EXAMPLE"},
		{"GitHub PAT", "ghp_aBcDeFgHiJkLmNoPqRsTuVwXyZ0123456789"},
		{"GitHub 细粒度 PAT", "github_pat_11ABCDEFG0123456789_abcdefghijklmnop"},
		{"Slack", "xoxb-123456789012-abcdefghijkl"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.True(t, adtyTextHasSecret(c.text))
		})
	}
}

// 这十条修前会被拒单拦住：讲配置的文档、贴出来的报错原文、待翻译的技术说明。
// 翻译那条拒单还是静默的（err=nil + Status=failed），所以这里的误伤代价最高。
func TestAdtyRejectPatternIgnoresConfigProse(t *testing.T) {
	cases := []struct {
		name string
		text string
	}{
		{"password 配置示例", "password: hunter2"},
		{"password 等号", "db password=changeme"},
		{"api_key 配置示例", "api_key: 8f3c2b1a9e7d6c5b"},
		{"api-key 等号", "api-key=abcdef123456"},
		{"验证码字段", "验证码: 8848"},
		{"风控公告", "本次风控升级会影响 api_key 的轮换流程，请提前准备。"},
		{"密码策略文档", "请在设置页点击 password 后方的箭头，再选择修改。"},
		{"JWT 技术文档", "JWT 由三段组成，第一段是 eyJhbGciOiJIUzI1NiJ9 这样的头。"},
		{"带 Bearer 的示例", "Authorization: Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9"},
		{"日志行号对", "task 123456:9f86d081884c7d659a2feaa0c55ad015 finished in 3.2s"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.False(t, adtyTextHasSecret(c.text), "正常文本不应触发拒单")
		})
	}
}

// 拆开的判据必须真的不同：同一段文本，脱敏要动、拒单不能动。这条断言就是
// 「一个形状不再承载两种东西」本身。
func TestAdtyRedactAndRejectServeDifferentTargets(t *testing.T) {
	text := "password: hunter2"
	redacted := redactAdtySecrets(text)
	assert.NotContains(t, redacted, "hunter2", "脱敏该宽")
	assert.Contains(t, redacted, "[redacted]")
	assert.False(t, adtyTextHasSecret(text), "拒单该窄——这是文档里的正常写法")

	key := "xoxb-123456789012-abcdefghijkl"
	assert.Contains(t, redactAdtySecrets(key), "[redacted]")
	assert.True(t, adtyTextHasSecret(key), "真凭据两套判据都要认")
}
