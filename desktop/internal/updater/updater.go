package updater

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"
)

const (
	RepositoryURL = "https://github.com/jmcgillcoder/USBridge"
	latestAPIURL  = "https://api.github.com/repos/jmcgillcoder/USBridge/releases/latest"
	maxAssetSize  = 200 << 20
)

var versionPattern = regexp.MustCompile(`(?i)^v?(\d+)\.(\d+)\.(\d+)`)

type Asset struct {
	Name string `json:"name"`
	URL  string `json:"browser_download_url"`
	Size int64  `json:"size"`
}

type releaseResponse struct {
	TagName     string    `json:"tag_name"`
	Name        string    `json:"name"`
	Body        string    `json:"body"`
	HTMLURL     string    `json:"html_url"`
	PublishedAt time.Time `json:"published_at"`
	Draft       bool      `json:"draft"`
	Prerelease  bool      `json:"prerelease"`
	Assets      []Asset   `json:"assets"`
}

type Info struct {
	CurrentVersion string `json:"currentVersion"`
	LatestVersion  string `json:"latestVersion"`
	Available      bool   `json:"available"`
	Name           string `json:"name,omitempty"`
	Notes          string `json:"notes,omitempty"`
	ReleaseURL     string `json:"releaseUrl,omitempty"`
	PublishedAt    string `json:"publishedAt,omitempty"`
	asset          Asset
	checksums      Asset
}

type Client struct {
	HTTPClient *http.Client
	APIURL     string
}

func NewClient() *Client {
	return &Client{
		HTTPClient: &http.Client{Timeout: 30 * time.Second},
		APIURL:     latestAPIURL,
	}
}

func (c *Client) Check(ctx context.Context, currentVersion string) (Info, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, c.APIURL, nil)
	if err != nil {
		return Info{}, err
	}
	request.Header.Set("Accept", "application/vnd.github+json")
	request.Header.Set("User-Agent", "USBridge/"+currentVersion)
	response, err := c.httpClient().Do(request)
	if err != nil {
		return Info{}, fmt.Errorf("连接 GitHub 失败：%w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return Info{}, fmt.Errorf("GitHub 返回了 HTTP %d", response.StatusCode)
	}
	var release releaseResponse
	if err := json.NewDecoder(io.LimitReader(response.Body, 2<<20)).Decode(&release); err != nil {
		return Info{}, fmt.Errorf("读取版本信息失败：%w", err)
	}
	if release.Draft || release.Prerelease {
		return Info{}, errors.New("最新版本尚未公开发布")
	}
	latest := strings.TrimPrefix(strings.TrimSpace(release.TagName), "v")
	if _, ok := parseVersion(latest); !ok {
		return Info{}, fmt.Errorf("无法识别版本号 %q", release.TagName)
	}
	info := Info{
		CurrentVersion: currentVersion,
		LatestVersion:  latest,
		Available:      compareVersions(latest, currentVersion) > 0,
		Name:           strings.TrimSpace(release.Name),
		Notes:          strings.TrimSpace(release.Body),
		ReleaseURL:     release.HTMLURL,
		PublishedAt:    release.PublishedAt.Format(time.RFC3339),
	}
	for _, asset := range release.Assets {
		lower := strings.ToLower(asset.Name)
		if lower == "sha256sums.txt" {
			info.checksums = asset
		}
		if runtime.GOOS == "windows" && runtime.GOARCH == "amd64" &&
			strings.HasPrefix(lower, "usbridge-windows-x64-") && strings.HasSuffix(lower, ".exe") {
			if info.asset.Name == "" || strings.HasSuffix(lower, "-signed.exe") {
				info.asset = asset
			}
		}
	}
	if info.Available && (info.asset.URL == "" || info.checksums.URL == "") {
		return Info{}, errors.New("该版本缺少 Windows 安装文件或校验文件")
	}
	return info, nil
}

func (c *Client) Download(ctx context.Context, info Info) (string, error) {
	if !info.Available || info.asset.URL == "" || info.checksums.URL == "" {
		return "", errors.New("没有可安装的新版本")
	}
	checksums, err := c.downloadBytes(ctx, info.checksums.URL, 2<<20)
	if err != nil {
		return "", fmt.Errorf("下载校验文件失败：%w", err)
	}
	expected, err := checksumFor(checksums, info.asset.Name)
	if err != nil {
		return "", err
	}
	directory, err := os.MkdirTemp("", "usbridge-update-*")
	if err != nil {
		return "", fmt.Errorf("创建更新目录失败：%w", err)
	}
	path := filepath.Join(directory, filepath.Base(info.asset.Name))
	if err := c.downloadFile(ctx, info.asset.URL, path, info.asset.Size); err != nil {
		os.RemoveAll(directory)
		return "", err
	}
	actual, err := fileSHA256(path)
	if err != nil {
		os.RemoveAll(directory)
		return "", fmt.Errorf("校验更新文件失败：%w", err)
	}
	if !strings.EqualFold(actual, expected) {
		os.RemoveAll(directory)
		return "", errors.New("更新文件校验失败，请稍后重试")
	}
	contents, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer contents.Close()
	header := make([]byte, 2)
	if _, err := io.ReadFull(contents, header); err != nil || string(header) != "MZ" {
		os.RemoveAll(directory)
		return "", errors.New("下载的文件不是有效的 Windows 程序")
	}
	return path, nil
}

func (c *Client) httpClient() *http.Client {
	if c.HTTPClient != nil {
		return c.HTTPClient
	}
	return http.DefaultClient
}

func (c *Client) downloadBytes(ctx context.Context, url string, limit int64) ([]byte, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("User-Agent", "USBridge-Updater")
	response, err := c.httpClient().Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", response.StatusCode)
	}
	return io.ReadAll(io.LimitReader(response.Body, limit))
}

func (c *Client) downloadFile(ctx context.Context, url, path string, declaredSize int64) error {
	if declaredSize > maxAssetSize {
		return errors.New("更新文件大小异常")
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	request.Header.Set("User-Agent", "USBridge-Updater")
	response, err := c.httpClient().Do(request)
	if err != nil {
		return fmt.Errorf("下载更新失败：%w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("下载更新失败：HTTP %d", response.StatusCode)
	}
	if response.ContentLength > maxAssetSize {
		return errors.New("更新文件大小异常")
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	written, copyErr := io.Copy(file, io.LimitReader(response.Body, maxAssetSize+1))
	closeErr := file.Close()
	if copyErr != nil {
		return fmt.Errorf("保存更新失败：%w", copyErr)
	}
	if closeErr != nil {
		return fmt.Errorf("保存更新失败：%w", closeErr)
	}
	if written > maxAssetSize || (declaredSize > 0 && written != declaredSize) {
		return errors.New("更新文件大小不匹配")
	}
	return nil
}

func checksumFor(contents []byte, name string) (string, error) {
	scanner := bufio.NewScanner(strings.NewReader(string(contents)))
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 2 {
			continue
		}
		candidate := strings.TrimPrefix(fields[len(fields)-1], "*")
		if filepath.Base(candidate) != filepath.Base(name) {
			continue
		}
		if len(fields[0]) != sha256.Size*2 {
			break
		}
		if _, err := hex.DecodeString(fields[0]); err != nil {
			break
		}
		return strings.ToLower(fields[0]), nil
	}
	return "", errors.New("校验文件中没有找到 Windows 安装文件")
}

func fileSHA256(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func parseVersion(value string) ([3]int, bool) {
	matches := versionPattern.FindStringSubmatch(strings.TrimSpace(value))
	if len(matches) != 4 {
		return [3]int{}, false
	}
	var result [3]int
	for index := range result {
		parsed, err := strconv.Atoi(matches[index+1])
		if err != nil {
			return [3]int{}, false
		}
		result[index] = parsed
	}
	return result, true
}

func compareVersions(left, right string) int {
	a, aOK := parseVersion(left)
	b, bOK := parseVersion(right)
	if !aOK || !bOK {
		return 0
	}
	for index := range a {
		if a[index] < b[index] {
			return -1
		}
		if a[index] > b[index] {
			return 1
		}
	}
	return 0
}
