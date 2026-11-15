package adty_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// 生产读的是 `deploy/adty/docker-compose.yml`，不是仓库根的 `docker-compose.yml`。
//
// 这条守的就是这件事：**一个变量加进了根 compose 不等于生产读得到。**
// `ISS-R4-0062` 的第一次修复就落在根 compose 上——文件里确实有那行、也确实在
// `upstream-touchpoints.md` 里登记了，而生产从来不读那个文件，线上仍然是死的。
//
// 所以判据是：**代码里读的每个 Telegram 开关，必须在生产 compose 的 environment 里
// 出现**；根 compose 有不算数。
func TestProductionComposeCarriesTelegramSwitch(t *testing.T) {
	root := repoRoot(t)
	compose := readFile(t, filepath.Join(root, "deploy", "adty", "docker-compose.yml"))

	if !strings.Contains(compose, "ADTY_TELEGRAM_ALLOW_NETWORK") {
		t.Fatalf("生产 compose 没有 ADTY_TELEGRAM_ALLOW_NETWORK：线上 AdtyTelegramSender 恒为 nil，\n" +
			"每条投递 Fail→dead、零日志，而「没有通知要发」和「一条都发不出去」外观一样。\n" +
			"（根目录 docker-compose.yml 是上游示例，生产不读它——见 docs/adty/deploy.md）")
	}

	// 必须是可被 .env 覆盖的引用形式，不是写死；否则要紧急停用就得改文件再重启。
	ref := regexp.MustCompile(`ADTY_TELEGRAM_ALLOW_NETWORK:\s*\$\{ADTY_TELEGRAM_ALLOW_NETWORK`)
	if !ref.MatchString(compose) {
		t.Fatalf("生产 compose 里的 Telegram 开关必须是 ${ADTY_TELEGRAM_ALLOW_NETWORK:-…} 形式，\n" +
			"写死的话紧急停用要改文件（客服/运维都提过这一条）")
	}
}

// 变量要有默认入口，否则运维不知道有这个开关。
func TestProductionEnvExampleDocumentsTelegramSwitch(t *testing.T) {
	root := repoRoot(t)
	env := readFile(t, filepath.Join(root, "deploy", "adty", ".env.example"))
	if !strings.Contains(env, "ADTY_TELEGRAM_ALLOW_NETWORK") {
		t.Fatal(".env.example 没有 ADTY_TELEGRAM_ALLOW_NETWORK：部署时没人会填它")
	}
}

// 反向守一条：**根 compose 不能当成证据。** 这里不断言它必须有什么，
// 只把「生产读哪个文件」这件事钉在测试里，免得下一个人又改错地方。
func TestProductionComposeIsTheDeployPathFile(t *testing.T) {
	root := repoRoot(t)
	deployDoc := readFile(t, filepath.Join(root, "docs", "adty", "deploy.md"))
	if !strings.Contains(deployDoc, "deploy/adty/") {
		t.Fatal("docs/adty/deploy.md 不再指向 deploy/adty/：生产 compose 的位置变了，这条测试的前提要重看")
	}
}

func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for i := 0; i < 6; i++ {
		if _, statErr := os.Stat(filepath.Join(dir, "go.mod")); statErr == nil {
			return dir
		}
		dir = filepath.Dir(dir)
	}
	t.Fatal("找不到仓库根")
	return ""
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(raw)
}
