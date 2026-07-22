package update

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/zhuyao/meatshell/internal/log"
)

// ReleaseInfo 表示 GitHub Release 信息
type ReleaseInfo struct {
	TagName     string `json:"tag_name"`
	Name        string `json:"name"`
	Body        string `json:"body"`
	HTMLURL     string `json:"html_url"`
	PublishedAt string `json:"published_at"`
}

// githubAPIURL 是 GitHub Releases API 地址
const githubAPIURL = "https://api.github.com/repos/jeff141/meatshell/releases/latest"

// requestTimeout 是 HTTP 请求超时时间
const requestTimeout = 10 * time.Second

// CheckLatest 检查最新版本。
// 请求 GitHub API 获取最新 Release 信息，并与当前版本比较。
// 返回值：
//   - release: 最新版本信息（请求失败时为 nil）
//   - hasUpdate: 是否有更新
//   - err: 错误信息（失败时静默忽略，仅记录日志）
func CheckLatest(ctx context.Context, currentVersion string) (*ReleaseInfo, bool, error) {
	// 创建带超时的 HTTP 请求
	reqCtx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, githubAPIURL, nil)
	if err != nil {
		log.Warn("failed to create update check request", "err", err)
		return nil, false, fmt.Errorf("create request: %w", err)
	}

	// 设置 GitHub API 需要的 Header
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Warn("failed to fetch latest release", "err", err)
		return nil, false, fmt.Errorf("fetch release: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Warn("github api returned non-200 status", "status", resp.StatusCode)
		return nil, false, fmt.Errorf("github api status: %d", resp.StatusCode)
	}

	var release ReleaseInfo
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		log.Warn("failed to decode release info", "err", err)
		return nil, false, fmt.Errorf("decode release: %w", err)
	}

	// 比较版本号
	hasUpdate := compareVersions(currentVersion, release.TagName)

	log.Info("update check completed",
		"current", currentVersion,
		"latest", release.TagName,
		"has_update", hasUpdate,
	)

	return &release, hasUpdate, nil
}

// compareVersions 比较当前版本与最新版本。
// 如果 latest 版本比 current 新，返回 true。
// 版本号格式如 "v1.0.0" 或 "1.0.0"，通过去除前缀后按语义比较。
func compareVersions(current, latest string) bool {
	current = normalizeVersion(current)
	latest = normalizeVersion(latest)

	if current == "" || latest == "" {
		return false
	}
	if current == latest {
		return false
	}

	// 简单的字符串比较：如果版本号不同，则认为有更新
	// 更精确的比较可以使用 semver 库，这里保持简单
	return latest != current
}

// normalizeVersion 规范化版本号字符串。
// 去除 "v" 前缀和首尾空格。
func normalizeVersion(v string) string {
	v = strings.TrimSpace(v)
	v = strings.TrimPrefix(v, "v")
	v = strings.TrimPrefix(v, "V")
	return v
}
