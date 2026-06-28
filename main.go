package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// ANSI Helpers (Standard 16-color palette only)
const (
	R = "\033[0m" // Reset
	B = "\033[1m" // Bold
	D = "\033[2m" // Dim
	I = "\033[3m" // Italic

	// Foreground accents (Standard 16 colors)
	FG_BLACK   = "\033[30m"
	FG_RED     = "\033[31m"
	FG_GREEN   = "\033[32m"
	FG_YELLOW  = "\033[33m"
	FG_BLUE    = "\033[34m"
	FG_MAGENTA = "\033[35m"
	FG_CYAN    = "\033[36m"
	FG_WHITE   = "\033[37m"

	FG_GRAY           = "\033[90m"
	FG_BRIGHT_RED     = "\033[91m"
	FG_BRIGHT_GREEN   = "\033[92m"
	FG_BRIGHT_YELLOW  = "\033[93m"
	FG_BRIGHT_BLUE    = "\033[94m"
	FG_BRIGHT_MAGENTA = "\033[95m"
	FG_BRIGHT_CYAN    = "\033[96m"
	FG_BRIGHT_WHITE   = "\033[97m"
)

var numColor = FG_BRIGHT_WHITE + B
var ansiRegex = regexp.MustCompile(`\x1b\[[0-9;]*m`)

type Model struct {
	ID          string `json:"id"`
	DisplayName string `json:"display_name"`
}

type CurrentUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

type ContextWindow struct {
	UsedPercentage      float64       `json:"used_percentage"`
	TotalInputTokens    int           `json:"total_input_tokens"`
	TotalOutputTokens   int           `json:"total_output_tokens"`
	ContextWindowSize   int           `json:"context_window_size"`
	RemainingPercentage float64       `json:"remaining_percentage"`
	CurrentUsage        *CurrentUsage `json:"current_usage"`
}

type VCS struct {
	Branch string `json:"branch"`
	Dirty  bool   `json:"dirty"`
	Type   string `json:"type"`
	Client string `json:"client"`
}

type Sandbox struct {
	Enabled      bool `json:"enabled"`
	AllowNetwork bool `json:"allow_network"`
}

type Cost struct {
	TotalCostUSD      float64 `json:"total_cost_usd"`
	TotalLinesAdded   int     `json:"total_lines_added"`
	TotalLinesRemoved int     `json:"total_lines_removed"`
}

type QuotaInfo struct {
	RemainingFraction *float64 `json:"remaining_fraction"`
	ResetInSeconds     *int     `json:"reset_in_seconds"`
}

type StatusInput struct {
	AgentState     string                `json:"agent_state"`
	ContextWindow  ContextWindow         `json:"context_window"`
	VCS            VCS                   `json:"vcs"`
	Sandbox        Sandbox               `json:"sandbox"`
	ArtifactCount  int                   `json:"artifact_count"`
	Subagents      []json.RawMessage     `json:"subagents"`
	TaskCount      int                   `json:"task_count"`
	Model          Model                 `json:"model"`
	TerminalWidth  int                   `json:"terminal_width"`
	CWD            string                `json:"cwd"`
	ConversationID string                `json:"conversation_id"`
	Product        string                `json:"product"`
	Quota          map[string]*QuotaInfo `json:"quota"`
	Version        string                `json:"version"`
	PlanTier       string                `json:"plan_tier"`
	Email          string                `json:"email"`
	Cost           Cost                  `json:"cost"`
}

type Theme struct {
	dotL1            string
	dotL2            string
	iconReady        string
	iconThinking     string
	iconWorking      string
	iconTool         string
	iconStateUnknown string
	iconVCS          string
	iconModel        string
	iconSandboxNet   string
	iconSandboxNoNet string
	iconSandboxOff   string
	iconContextBar   string
	iconArtifacts    string
	iconSubagents    string
	iconTasks        string
	iconDir          string
	iconConv         string
	iconTokSum       string
	iconReset        string
	iconAC           string
	iconBat          string
}

func getTheme(classic bool) Theme {
	if classic {
		return Theme{
			dotL1:            FG_GRAY + " ╱ " + R,
			dotL2:            FG_GRAY + " · " + R,
			iconReady:        "●",
			iconThinking:     "◆",
			iconWorking:      "⚙",
			iconTool:         "🔧",
			iconStateUnknown: "⏳",
			iconVCS:          "╱",
			iconModel:        "",
			iconSandboxNet:   "ON (net)",
			iconSandboxNoNet: "ON (no-net)",
			iconSandboxOff:   "OFF",
			iconContextBar:   "ctx",
			iconArtifacts:    "artifacts",
			iconSubagents:    "subagents",
			iconTasks:        "tasks",
			iconDir:          "╱",
			iconConv:         "╱",
			iconTokSum:       "",
			iconReset:        "⌛",
			iconAC:           "AC",
			iconBat:          "BAT",
		}
	}
	return Theme{
		dotL1:            FG_GRAY + " | " + R,
		dotL2:            FG_GRAY + " | " + R,
		iconReady:        "",
		iconThinking:     "󰟷",
		iconWorking:      "",
		iconTool:         "",
		iconStateUnknown: "",
		iconVCS:          "",
		iconModel:        "",
		iconSandboxNet:   "󰒙",
		iconSandboxNoNet: "󰴴",
		iconSandboxOff:   "󰦜",
		iconContextBar:   "󱍏",
		iconArtifacts:    "",
		iconSubagents:    "󱙺",
		iconTasks:        "",
		iconDir:          "",
		iconConv:         "󰍪",
		iconTokSum:       "",
		iconReset:        "⌛️",
		iconAC:           "󰚥",
		iconBat:          "🔋",
	}
}

func getGitInfo(cwd string) (string, bool) {
	if cwd == "" {
		cwd = "."
	}
	cmd := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
	cmd.Dir = cwd
	out, err := cmd.Output()
	if err != nil {
		return "", false
	}
	branch := strings.TrimSpace(string(out))
	if branch == "" {
		return "", false
	}

	cmdDirty := exec.Command("git", "status", "--porcelain")
	cmdDirty.Dir = cwd
	outDirty, errDirty := cmdDirty.Output()
	dirty := false
	if errDirty == nil && len(bytes.TrimSpace(outDirty)) > 0 {
		dirty = true
	}
	return branch, dirty
}

// getGitLOC returns the total lines added and removed across all uncommitted
// changes (staged + unstaged) relative to HEAD, by parsing `git diff --numstat`.
func getGitLOC(cwd string) (added int, removed int) {
	if cwd == "" {
		cwd = "."
	}
	// Include both staged and unstaged changes.
	for _, args := range [][]string{
		{"diff", "--numstat", "HEAD"},        // all changes vs HEAD
		{"diff", "--numstat", "--cached"},    // staged only (fallback for new repos)
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = cwd
		out, err := cmd.Output()
		if err != nil {
			continue
		}
		for _, line := range strings.Split(string(out), "\n") {
			fields := strings.Fields(line)
			if len(fields) < 2 {
				continue
			}
			// Binary files show "-" — skip them.
			if fields[0] == "-" || fields[1] == "-" {
				continue
			}
			var a, r int
			fmt.Sscan(fields[0], &a)
			fmt.Sscan(fields[1], &r)
			added += a
			removed += r
		}
		if added > 0 || removed > 0 {
			break // got useful data, don't double-count
		}
	}
	return
}

func getTailscaleIP() string {
	ifaces, err := net.Interfaces()
	if err == nil {
		for _, iface := range ifaces {
			nameLower := strings.ToLower(iface.Name)
			if iface.Name == "tailscale0" || strings.Contains(nameLower, "tailscale") || strings.Contains(nameLower, "ts0") {
				addrs, err := iface.Addrs()
				if err == nil {
					for _, addr := range addrs {
						var ip net.IP
						switch v := addr.(type) {
						case *net.IPNet:
							ip = v.IP
						case *net.IPAddr:
							ip = v.IP
						}
						if ip != nil && ip.To4() != nil {
							return ip.String()
						}
					}
				}
			}
		}
	}
	cmd := exec.Command("tailscale", "ip", "-4")
	out, err := cmd.Output()
	if err == nil {
		return strings.TrimSpace(string(out))
	}
	return ""
}

func humanFormat(num int) string {
	if num == 0 {
		return "0"
	}
	if num >= 1000000 {
		val := float64(num) / 1000000.0
		return fmt.Sprintf("%.1fM", val)
	}
	if num >= 1000 {
		val := float64(num) / 1000.0
		return fmt.Sprintf("%.1fK", val)
	}
	return strconv.Itoa(num)
}

// estimateCostUSD calculates a USD cost estimate from token counts using a
// model pricing table keyed on model ID substrings. All prices are per 1M tokens.
// Returns 0 if the model is not found or no tokens have been used.
func estimateCostUSD(modelID string, inputTokens, outputTokens int) float64 {
	type modelPrice struct {
		substr    string
		inputPPM  float64 // price per 1M input tokens
		outputPPM float64 // price per 1M output tokens
	}
	// Ordered most-specific first so longer substrings match before shorter ones.
	pricing := []modelPrice{
		// ── Gemini 3.x ─────────────────────────────────────────────────────────
		{"gemini-3.5-flash", 1.50, 9.00},
		{"gemini-3.5-pro", 7.00, 21.00},
		{"gemini-3.1-flash-lite", 0.25, 1.50},
		{"gemini-3.1-flash", 0.50, 3.00},
		{"gemini-3.1-pro", 3.50, 10.50},
		{"gemini-3-flash", 0.50, 3.00},
		{"gemini-3-pro", 3.50, 10.50},
		// ── Gemini 2.5 / 2.0 / 1.5 ────────────────────────────────────────────
		{"gemini-2.5-flash", 0.30, 2.50},
		{"gemini-2.5-pro", 1.25, 10.00},
		{"gemini-2.0-flash-lite", 0.075, 0.30},
		{"gemini-2.0-flash", 0.10, 0.40},
		{"gemini-2.0-pro", 1.25, 5.00},
		{"gemini-1.5-flash-8b", 0.0375, 0.15},
		{"gemini-1.5-flash", 0.075, 0.30},
		{"gemini-1.5-pro", 1.25, 5.00},
		// ── Claude (Anthropic) ─────────────────────────────────────────────────
		// Claude 4.x / 5.x — most-specific variants first
		{"claude-fable", 10.00, 50.00},
		{"claude-opus", 5.00, 25.00},
		{"claude-sonnet", 3.00, 15.00},
		{"claude-haiku", 1.00, 5.00},
		// Generic fallback for any unknown claude model
		{"claude", 3.00, 15.00},
		// ── OpenAI / GPT ───────────────────────────────────────────────────────
		{"gpt-4o-mini", 0.15, 0.60},
		{"gpt-4o", 2.50, 10.00},
		{"gpt-4-turbo", 10.00, 30.00},
		{"gpt-4", 30.00, 60.00},
		{"gpt-3.5", 0.50, 1.50},
		{"o3-mini", 1.10, 4.40},
		{"o3", 10.00, 40.00},
		{"o1-mini", 3.00, 12.00},
		{"o1", 15.00, 60.00},
	}

	lowerID := strings.ToLower(modelID)
	for _, p := range pricing {
		if strings.Contains(lowerID, p.substr) {
			in := float64(inputTokens) / 1_000_000.0 * p.inputPPM
			out := float64(outputTokens) / 1_000_000.0 * p.outputPPM
			return in + out
		}
	}
	return 0
}

// knownContextWindowSize returns a best-known context window token limit for a
// given model ID, used when the CLI doesn't report ContextWindowSize (e.g. for
// non-Gemini models). Returns 0 if the model is unrecognised.
func knownContextWindowSize(modelID string) int {
	type ctxEntry struct {
		substr string
		size   int
	}
	// Most-specific first.
	table := []ctxEntry{
		// Claude 4.x+ — 1M context
		{"claude-fable", 1_000_000},
		{"claude-opus", 1_000_000},
		{"claude-sonnet", 1_000_000},
		{"claude-haiku", 1_000_000},
		{"claude", 200_000},
		// OpenAI / GPT
		{"gpt-4o-mini", 128_000},
		{"gpt-4o", 128_000},
		{"gpt-4-turbo", 128_000},
		{"gpt-4", 8_192},
		{"gpt-3.5-turbo-16k", 16_384},
		{"gpt-3.5", 4_096},
		{"o3-mini", 200_000},
		{"o3", 200_000},
		{"o1-mini", 128_000},
		{"o1", 200_000},
		// Gemini (as fallback if CLI omits the field)
		{"gemini-3", 2_000_000},
		{"gemini-2.5", 1_000_000},
		{"gemini-2.0-flash-lite", 1_000_000},
		{"gemini-2.0", 1_000_000},
		{"gemini-1.5-pro", 2_000_000},
		{"gemini-1.5-flash", 1_000_000},
	}

	lowerID := strings.ToLower(modelID)
	for _, e := range table {
		if strings.Contains(lowerID, e.substr) {
			return e.size
		}
	}
	return 0
}

func shortenPath(path string) string {
	if path == "" {
		return ""
	}
	home, err := os.UserHomeDir()
	if err == nil {
		if strings.HasPrefix(path, home) {
			path = "~" + strings.TrimPrefix(path, home)
		}
	}
	if len(path) > 25 {
		return "..." + filepath.Base(path)
	}
	return path
}

func visibleLen(s string) int {
	stripped := ansiRegex.ReplaceAllString(s, "")
	return len([]rune(stripped))
}

func formatResetTime(sec int) string {
	if sec <= 0 {
		return ""
	}
	days := sec / 86400
	rem := sec % 86400
	hours := rem / 3600
	rem = rem % 3600
	mins := rem / 60

	if days > 0 {
		if hours > 0 {
			return fmt.Sprintf("%dd %dh", days, hours)
		}
		return fmt.Sprintf("%dd", days)
	}
	if hours > 0 {
		if mins > 0 {
			return fmt.Sprintf("%dh %dm", hours, mins)
		}
		return fmt.Sprintf("%dh", hours)
	}
	if mins > 0 {
		return fmt.Sprintf("%dm", mins)
	}
	return "<1m"
}

func makeQuotaBar(theme Theme, val float64, label string, barColor string, resetSec int) string {
	if val < 0 {
		return FG_BRIGHT_WHITE + B + label + R + " " + FG_GRAY + "N/A" + R
	}

	valInt := int(val)
	textColor := FG_BRIGHT_GREEN
	if valInt < 20 {
		textColor = FG_BRIGHT_RED
	} else if valInt < 50 {
		textColor = FG_BRIGHT_YELLOW
	}

	resetStr := ""
	t := formatResetTime(resetSec)
	if t != "" {
		resetStr = " " + theme.iconReset + " " + t
	}

	valFmt := fmt.Sprintf("%.2f", val)
	return FG_BRIGHT_WHITE + B + label + R + " " + textColor + valFmt + "%" + R + resetStr
}

func osc8(url, label string) string {
	return fmt.Sprintf("\033]8;;%s\033\\%s\033]8;;\033\\", url, label)
}

func getGitRepoInfo(cwd string) (string, string, string) {
	if cwd == "" {
		cwd = "."
	}
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	cmd.Dir = cwd
	out, err := cmd.Output()
	if err != nil {
		return "", "", ""
	}
	repoRoot := strings.TrimSpace(string(out))
	if repoRoot == "" {
		return "", "", ""
	}

	repoName := filepath.Base(repoRoot)
	rel, err := filepath.Rel(repoRoot, cwd)
	if err != nil || rel == "." {
		rel = ""
	}
	return repoRoot, repoName, rel
}

func getGitRemoteURL(cwd string) string {
	if cwd == "" {
		cwd = "."
	}
	cmd := exec.Command("git", "config", "--get", "remote.origin.url")
	cmd.Dir = cwd
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	rawURL := strings.TrimSpace(string(out))
	if rawURL == "" {
		return ""
	}

	if strings.HasPrefix(rawURL, "git@") {
		parts := strings.SplitN(rawURL, "@", 2)
		if len(parts) == 2 {
			rest := parts[1]
			rest = strings.Replace(rest, ":", "/", 1)
			rawURL = "https://" + rest
		}
	}
	rawURL = strings.TrimSuffix(rawURL, ".git")
	return rawURL
}

func getGitBranchURL(remoteURL, branch string) string {
	if remoteURL == "" || branch == "" {
		return remoteURL
	}
	if strings.Contains(remoteURL, "github.com") || strings.Contains(remoteURL, "gitlab.com") {
		return remoteURL + "/tree/" + branch
	}
	if strings.Contains(remoteURL, "bitbucket.org") {
		return remoteURL + "/src/" + branch
	}
	return remoteURL + "/tree/" + branch
}

func formatDirectorySegment(theme Theme, cwd string, useClassicIcons bool) string {
	cwdShort := shortenPath(cwd)
	dirLink := osc8("file://"+cwd, cwdShort)
	dirPart := theme.iconDir + " "
	result := theme.dotL1 + FG_CYAN + dirPart + dirLink + R

	repoRoot, repoName, _ := getGitRepoInfo(cwd)
	if repoRoot != "" {
		branch, dirty := getGitInfo(cwd)
		if branch != "" {
			remoteURL := getGitRemoteURL(cwd)
			var repoLink string
			label := repoName + "@" + branch
			if dirty {
				label += "*"
			}

			if remoteURL != "" {
				branchURL := getGitBranchURL(remoteURL, branch)
				repoLink = osc8(branchURL, label)
			} else {
				repoLink = osc8("file://"+repoRoot, label)
			}

			color := FG_BLUE
			if dirty {
				color = FG_BRIGHT_RED
			}

			if useClassicIcons {
				result += color + " ╱ " + repoLink + R
			} else {
				result += " " + color + theme.iconVCS + " " + repoLink + R
			}
		}
	}
	return result
}

func joinAndWrap(theme Theme, parts []string, separator string, totalCols int, prefix string) string {
	var lines []string
	currentLine := ""
	currentLen := visibleLen(prefix)

	for _, part := range parts {
		if part == "" {
			continue
		}
		partLen := visibleLen(part)
		sepLen := visibleLen(separator)

		addedLen := partLen
		addedStr := part
		if currentLine != "" {
			addedLen += sepLen
			addedStr = separator + part
		}

		if currentLen+addedLen > totalCols && currentLine != "" {
			lines = append(lines, currentLine)
			// Strip leading separators from the wrapped line start
			cleanPart := part
			cleanPart = strings.TrimPrefix(cleanPart, theme.dotL1)
			cleanPart = strings.TrimPrefix(cleanPart, theme.dotL2)
			cleanPart = strings.TrimPrefix(cleanPart, " ")

			currentLine = cleanPart
			currentLen = visibleLen(prefix) + visibleLen(cleanPart)
		} else {
			currentLine += addedStr
			currentLen += addedLen
		}
	}
	if currentLine != "" {
		lines = append(lines, currentLine)
	}
	return strings.Join(lines, "\n"+prefix)
}

var commitHash = ""
const statuslineVersion = "v0.0.1"

func getStatuslineVersion() string {
	if commitHash != "" {
		return commitHash
	}
	return statuslineVersion
}

type UpdateCache struct {
	LatestVersion string `json:"latest_version"`
	CheckedAt     int64  `json:"checked_at"`
}

type GithubRelease struct {
	TagName string `json:"tag_name"`
}

type GithubAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

type GithubReleaseDetailed struct {
	TagName string        `json:"tag_name"`
	Assets  []GithubAsset `json:"assets"`
}

func getUpdateCachePath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	// Create directory if missing
	dir := filepath.Join(home, ".antigravity")
	_ = os.MkdirAll(dir, 0755)
	return filepath.Join(dir, ".latest_version_check")
}

func isNewerVersion(current, latest string) bool {
	c := strings.TrimPrefix(current, "v")
	l := strings.TrimPrefix(latest, "v")
	return c != l && l != ""
}

func checkUpdateBackground() {
	self, err := os.Executable()
	if err != nil {
		return
	}
	cmd := exec.Command(self, "--check-update")
	cmd.Stdout = nil
	cmd.Stderr = nil
	cmd.Stdin = nil
	_ = cmd.Start()
}

func runCheckUpdate() {
	cachePath := getUpdateCachePath()
	if cachePath == "" {
		return
	}

	s := loadSettings()
	var targetURL string
	if s.UpdateBranch == "canary" {
		targetURL = "https://api.github.com/repos/bradly0cjw/agycli-statusline/commits/main"
	} else {
		targetURL = "https://api.github.com/repos/bradly0cjw/agycli-statusline/releases/latest"
	}

	client := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequest("GET", targetURL, nil)
	if err != nil {
		return
	}
	req.Header.Set("User-Agent", "antigravity-cli-statusline-update-checker")

	resp, err := client.Do(req)
	if err != nil {
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return
	}

	var latestVersion string
	if s.UpdateBranch == "canary" {
		type GithubCommit struct {
			SHA string `json:"sha"`
		}
		var commit GithubCommit
		if err := json.NewDecoder(resp.Body).Decode(&commit); err == nil && len(commit.SHA) > 7 {
			latestVersion = commit.SHA[:7]
		}
	} else {
		var release GithubRelease
		if err := json.NewDecoder(resp.Body).Decode(&release); err == nil {
			latestVersion = release.TagName
		}
	}

	if latestVersion == "" {
		return
	}

	cache := UpdateCache{
		LatestVersion: latestVersion,
		CheckedAt:     time.Now().Unix(),
	}

	data, err := json.Marshal(cache)
	if err == nil {
		_ = os.WriteFile(cachePath, data, 0644)
	}
}

func runSelfUpdate() {
	s := loadSettings()
	if s.UpdateBranch == "canary" {
		runCanarySelfUpdate()
		return
	}
	runReleaseSelfUpdate()
}

func runCanarySelfUpdate() {
	fmt.Println("Checking for latest canary commits from GitHub...")
	client := &http.Client{Timeout: 15 * time.Second}
	req, err := http.NewRequest("GET", "https://api.github.com/repos/bradly0cjw/agycli-statusline/commits/main", nil)
	if err != nil {
		fmt.Printf("Error: Failed to create request: %v\n", err)
		os.Exit(1)
	}
	req.Header.Set("User-Agent", "antigravity-cli-statusline-self-updater")

	resp, err := client.Do(req)
	if err != nil {
		fmt.Printf("Error: Failed to check updates: %v\n", err)
		os.Exit(1)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		fmt.Printf("Error: Received HTTP %d from GitHub\n", resp.StatusCode)
		os.Exit(1)
	}

	type GithubCommit struct {
		SHA string `json:"sha"`
	}
	var commit GithubCommit
	if err := json.NewDecoder(resp.Body).Decode(&commit); err != nil || len(commit.SHA) < 7 {
		fmt.Printf("Error: Failed to parse GitHub commit details: %v\n", err)
		os.Exit(1)
	}
	latestSHA := commit.SHA[:7]

	if !isNewerVersion(getStatuslineVersion(), latestSHA) {
		fmt.Printf("Statusline is already up-to-date with canary (current: %s, latest: %s).\n", getStatuslineVersion(), latestSHA)
		return
	}

	fmt.Printf("New canary commit %s is available (current: %s).\n", latestSHA, getStatuslineVersion())
	fmt.Println("Cloning latest main branch and compiling locally...")

	tempDir, err := os.MkdirTemp("", "statusline-canary-*")
	if err != nil {
		fmt.Printf("Error: Failed to create temporary build directory: %v\n", err)
		os.Exit(1)
	}
	defer os.RemoveAll(tempDir)

	cloneCmd := exec.Command("git", "clone", "--depth", "1", "https://github.com/bradly0cjw/agycli-statusline.git", tempDir)
	if out, err := cloneCmd.CombinedOutput(); err != nil {
		fmt.Printf("Error: Failed to clone repository: %v\nOutput: %s\n", err, string(out))
		os.Exit(1)
	}

	selfPath, err := os.Executable()
	if err != nil {
		fmt.Printf("Error: Failed to locate current executable path: %v\n", err)
		os.Exit(1)
	}

	tempBinaryPath := filepath.Join(tempDir, "statusline_new")
	if runtime.GOOS == "windows" {
		tempBinaryPath += ".exe"
	}

	buildCmd := exec.Command("go", "build", "-ldflags", fmt.Sprintf("-s -w -X main.commitHash=%s", latestSHA), "-o", tempBinaryPath)
	buildCmd.Dir = tempDir
	if out, err := buildCmd.CombinedOutput(); err != nil {
		fmt.Printf("Error: Failed to compile new statusline binary: %v\nOutput: %s\n", err, string(out))
		os.Exit(1)
	}

	if runtime.GOOS == "windows" {
		bakPath := selfPath + ".bak"
		_ = os.Remove(bakPath)
		err = os.Rename(selfPath, bakPath)
		if err != nil {
			fmt.Printf("Error: Failed to rename running executable: %v\n", err)
			os.Exit(1)
		}
		err = os.Rename(tempBinaryPath, selfPath)
		if err != nil {
			_ = os.Rename(bakPath, selfPath)
			fmt.Printf("Error: Failed to deploy new executable: %v\n", err)
			os.Exit(1)
		}
	} else {
		err = os.Rename(tempBinaryPath, selfPath)
		if err != nil {
			fmt.Printf("Error: Failed to deploy new executable: %v\n", err)
			os.Exit(1)
		}
		_ = os.Chmod(selfPath, 0755)
	}

	fmt.Printf("🎉 Successfully updated Antigravity CLI statusline to canary commit %s!\n", latestSHA)
}

func runReleaseSelfUpdate() {
	fmt.Println("Checking for updates from GitHub...")
	client := &http.Client{Timeout: 15 * time.Second}
	req, err := http.NewRequest("GET", "https://api.github.com/repos/bradly0cjw/agycli-statusline/releases/latest", nil)
	if err != nil {
		fmt.Printf("Error: Failed to create update request: %v\n", err)
		os.Exit(1)
	}
	req.Header.Set("User-Agent", "antigravity-cli-statusline-self-updater")

	resp, err := client.Do(req)
	if err != nil {
		fmt.Printf("Error: Failed to check updates: %v\n", err)
		os.Exit(1)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		fmt.Printf("Error: Received HTTP %d from GitHub\n", resp.StatusCode)
		os.Exit(1)
	}

	var release GithubReleaseDetailed
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		fmt.Printf("Error: Failed to parse GitHub release details: %v\n", err)
		os.Exit(1)
	}

	if !isNewerVersion(getStatuslineVersion(), release.TagName) {
		fmt.Printf("Statusline is already up-to-date (current: %s, latest: %s).\n", getStatuslineVersion(), release.TagName)
		return
	}

	fmt.Printf("New version %s is available (current: %s).\n", release.TagName, statuslineVersion)

	var downloadURL string
	var assetName string
	goos := runtime.GOOS
	goarch := runtime.GOARCH

	for _, asset := range release.Assets {
		nameLower := strings.ToLower(asset.Name)
		if strings.Contains(nameLower, goos) && (strings.Contains(nameLower, goarch) || (goarch == "amd64" && strings.Contains(nameLower, "x86_64"))) {
			downloadURL = asset.BrowserDownloadURL
			assetName = asset.Name
			break
		}
	}

	if downloadURL == "" {
		fmt.Printf("Error: No pre-built release asset found for OS: %s, Arch: %s.\n", goos, goarch)
		fmt.Println("Please run install.sh / install.ps1 to rebuild locally from source.")
		os.Exit(1)
	}

	fmt.Printf("Downloading update asset: %s...\n", assetName)
	reqDownload, err := http.NewRequest("GET", downloadURL, nil)
	if err != nil {
		fmt.Printf("Error: Failed to create download request: %v\n", err)
		os.Exit(1)
	}
	reqDownload.Header.Set("User-Agent", "antigravity-cli-statusline-self-updater")

	respDownload, err := client.Do(reqDownload)
	if err != nil {
		fmt.Printf("Error: Failed to download asset: %v\n", err)
		os.Exit(1)
	}
	defer respDownload.Body.Close()

	if respDownload.StatusCode != http.StatusOK {
		fmt.Printf("Error: Failed to download asset: HTTP %d\n", respDownload.StatusCode)
		os.Exit(1)
	}

	selfPath, err := os.Executable()
	if err != nil {
		fmt.Printf("Error: Failed to locate current executable path: %v\n", err)
		os.Exit(1)
	}

	tmpFile, err := os.CreateTemp("", "statusline-update-*")
	if err != nil {
		fmt.Printf("Error: Failed to create temporary file: %v\n", err)
		os.Exit(1)
	}
	tmpPath := tmpFile.Name()
	defer os.Remove(tmpPath)

	_, err = io.Copy(tmpFile, respDownload.Body)
	tmpFile.Close()
	if err != nil {
		fmt.Printf("Error: Failed to write downloaded update: %v\n", err)
		os.Exit(1)
	}

	if runtime.GOOS == "windows" {
		bakPath := selfPath + ".bak"
		_ = os.Remove(bakPath)
		err = os.Rename(selfPath, bakPath)
		if err != nil {
			fmt.Printf("Error: Failed to rename running executable: %v\n", err)
			os.Exit(1)
		}
		err = os.Rename(tmpPath, selfPath)
		if err != nil {
			// Rollback
			_ = os.Rename(bakPath, selfPath)
			fmt.Printf("Error: Failed to deploy new executable: %v\n", err)
			os.Exit(1)
		}
	} else {
		err = os.Rename(tmpPath, selfPath)
		if err != nil {
			fmt.Printf("Error: Failed to deploy new executable: %v\n", err)
			os.Exit(1)
		}
		_ = os.Chmod(selfPath, 0755)
	}

	fmt.Printf("🎉 Successfully updated Antigravity CLI statusline to %s!\n", release.TagName)
}

func getStatuslineConfigPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	dir := filepath.Join(home, ".antigravity")
	_ = os.MkdirAll(dir, 0755)
	return filepath.Join(dir, "config.json")
}

type Settings struct {
	Options      map[string]bool `json:"options"`
	UpdateBranch string          `json:"updateBranch"`
}

func loadSettings() Settings {
	defaultOptions := map[string]bool{
		"showVersion": true,
		"showUser":    true,
		"showHost":    true,
		"showModel":   true,
		"showDir":     true,
		"showStats":   true,
		"showCost":    true,
		"showSandbox": true,
		"showTokens":  true,
		"showQuotas":  true,
		"showPower":   true,
	}

	path := getStatuslineConfigPath()
	if path == "" {
		return Settings{Options: defaultOptions, UpdateBranch: "release"}
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return Settings{Options: defaultOptions, UpdateBranch: "release"}
	}

	var s Settings
	if err := json.Unmarshal(data, &s); err != nil {
		var oldOptions map[string]bool
		if json.Unmarshal(data, &oldOptions) == nil {
			for k, v := range oldOptions {
				if _, exists := defaultOptions[k]; exists {
					defaultOptions[k] = v
				}
			}
		}
		return Settings{Options: defaultOptions, UpdateBranch: "release"}
	}

	for k, v := range s.Options {
		if _, exists := defaultOptions[k]; exists {
			defaultOptions[k] = v
		}
	}
	s.Options = defaultOptions
	if s.UpdateBranch != "release" && s.UpdateBranch != "canary" {
		s.UpdateBranch = "release"
	}

	return s
}

func saveSettings(s Settings) error {
	path := getStatuslineConfigPath()
	if path == "" {
		return fmt.Errorf("could not resolve home directory")
	}

	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0644)
}

func runInteractiveSettings() {
	settings := loadSettings()
	theme := getTheme(false)

	keys := []string{
		"showVersion",
		"showUser",
		"showHost",
		"showModel",
		"showDir",
		"showStats",
		"showCost",
		"showSandbox",
		"showTokens",
		"showQuotas",
		"showPower",
	}

	labels := map[string]string{
		"showVersion": "Show CLI & Statusline Version",
		"showUser":    "Show User Info (Plan / Email)",
		"showHost":    "Show Host details (Hostname / Tailscale IP)",
		"showModel":   "Show active Model Name",
		"showDir":     "Show CWD Directory & Repository (Hyperlinks)",
		"showStats":   "Show Telemetry counters (Artifacts / Subagents / Tasks)",
		"showCost":    "Show Session Cost ($) & LOC Diff (+/-)",
		"showSandbox": "Show Sandbox status Badge",
		"showTokens":  "Show Token meters & usage percentage",
		"showQuotas":  "Show active Quota meters",
		"showPower":   "Show Power / Battery / AC status",
	}

	for {
		fmt.Print("\033[H\033[2J") // Clear screen using ANSI escape codes
		fmt.Println("====================================================")
		fmt.Println("       Antigravity CLI Statusline Settings         ")
		fmt.Println("====================================================")
		for i, key := range keys {
			status := "OFF"
			if settings.Options[key] {
				status = "ON "
			}
			color := FG_BRIGHT_RED // Red for OFF
			if settings.Options[key] {
				color = FG_BRIGHT_GREEN // Green for ON
			}
			fmt.Printf(" %2d. [%s%s%s] %s\n", i+1, color, status, R, labels[key])
		}
		// Add Update track toggle as Option 12
		trackStatus := "release"
		if settings.UpdateBranch == "canary" {
			trackStatus = "canary "
		}
		trackColor := FG_BRIGHT_GREEN
		if settings.UpdateBranch == "canary" {
			trackColor = FG_BRIGHT_YELLOW
		}
		fmt.Printf(" 12. [%s%s%s] Update Track (toggle release/canary)\n", trackColor, trackStatus, R)

		// Add immediate update check option as option 13
		fmt.Printf(" 13. [\033[36mACTION\033[0m] Check GitHub for Updates Now\n")
		fmt.Println("====================================================")
		fmt.Println(" Enter number to toggle, 's' to save & exit, or 'q' to quit.")
		fmt.Print(" Action: ")

		var input string
		_, err := fmt.Scanln(&input)
		if err != nil {
			continue
		}

		input = strings.TrimSpace(strings.ToLower(input))
		if input == "s" {
			err := saveSettings(settings)
			if err != nil {
				fmt.Printf("\033[31mError saving settings: %v\033[0m\n", err)
				time.Sleep(2 * time.Second)
			} else {
				fmt.Println("\033[32mSettings saved successfully!\033[0m")
				time.Sleep(1 * time.Second)
			}
			break
		}
		if input == "q" {
			break
		}

		if input == "12" {
			if settings.UpdateBranch == "release" {
				settings.UpdateBranch = "canary"
			} else {
				settings.UpdateBranch = "release"
			}
			continue
		}

		if input == "13" {
			fmt.Println("\033[36mChecking GitHub for updates...\033[0m")
			runCheckUpdate()
			// Read cache to see latest version
			cachePath := getUpdateCachePath()
			if fileData, err := os.ReadFile(cachePath); err == nil {
				var cache UpdateCache
				if json.Unmarshal(fileData, &cache) == nil {
					if isNewerVersion(getStatuslineVersion(), cache.LatestVersion) {
						fmt.Printf("\033[33mUpdate available: %s (current: %s)!\033[0m\n", cache.LatestVersion, getStatuslineVersion())
					} else {
						fmt.Printf("\033[32mStatusline is up-to-date (version: %s).\033[0m\n", getStatuslineVersion())
					}
				}
			}
			time.Sleep(2 * time.Second)
			continue
		}

		idx, err := strconv.Atoi(input)
		if err == nil && idx >= 1 && idx <= len(keys) {
			key := keys[idx-1]
			settings.Options[key] = !settings.Options[key]
		}
	}
	_ = theme // Avoid unused warning
}

func printRightAligned(left, right string, totalCols int) string {
	leftVis := visibleLen(left)
	rightVis := visibleLen(right)
	pad := totalCols - leftVis - rightVis
	if pad < 1 {
		pad = 1
	}
	return left + strings.Repeat(" ", pad) + right
}

func main() {
	for _, arg := range os.Args[1:] {
		if arg == "--settings" || arg == "--config" {
			runInteractiveSettings()
			return
		}
		if arg == "--check-update" {
			runCheckUpdate()
			return
		}
		if arg == "--update" {
			runSelfUpdate()
			return
		}
		if arg == "--refresh" || arg == "-r" {
			fmt.Println("Refreshing update cache from GitHub...")
			runCheckUpdate()
			fmt.Println("Done.")
			return
		}
	}

	useClassicIcons := false
	for _, arg := range os.Args[1:] {
		if arg == "--classic" || arg == "--no-nerdfont" || arg == "--compatibility" {
			useClassicIcons = true
		}
	}

	// Read update check cache and load options
	settings := loadSettings()
	options := settings.Options
	updateAvailable := ""
	cachePath := getUpdateCachePath()
	if cachePath != "" {
		if fileData, err := os.ReadFile(cachePath); err == nil {
			var cache UpdateCache
			if json.Unmarshal(fileData, &cache) == nil {
				if isNewerVersion(getStatuslineVersion(), cache.LatestVersion) {
					updateAvailable = cache.LatestVersion
				}
				if time.Now().Unix()-cache.CheckedAt > 86400 {
					checkUpdateBackground()
				}
			} else {
				checkUpdateBackground()
			}
		} else {
			checkUpdateBackground()
		}
	}

	theme := getTheme(useClassicIcons)

	// Read input from stdin
	data, err := io.ReadAll(os.Stdin)
	if err != nil {
		// Output empty statusline on error
		return
	}

	var input StatusInput
	// Unmarshal input safely, with default fallbacks
	if err := json.Unmarshal(data, &input); err != nil {
		return
	}

	if input.AgentState == "" {
		input.AgentState = "idle"
	}
	if input.TerminalWidth <= 0 {
		input.TerminalWidth = 80
	}

	// ─── VCS directly from git (bypasses JSON caching) ───────────────────────
	gitBranch, gitDirty := getGitInfo(input.CWD)
	if gitBranch != "" {
		input.VCS.Branch = gitBranch
		input.VCS.Dirty = gitDirty
		input.VCS.Type = "git"
	} else {
		input.VCS.Branch = ""
		input.VCS.Dirty = false
		input.VCS.Type = ""
	}

	// ─── Computed values ─────────────────────────────────────────────────────
	// UsedPercentage is only reported by Gemini-backed sessions. For other models
	// (Claude, GPT, etc.) we derive it from token counts and a known context window.
	usedPct := input.ContextWindow.UsedPercentage
	if usedPct == 0 && input.ContextWindow.ContextWindowSize == 0 {
		// Try to infer context window size from the model ID.
		input.ContextWindow.ContextWindowSize = knownContextWindowSize(input.Model.ID)
	}
	if usedPct == 0 && input.ContextWindow.ContextWindowSize > 0 {
		totalUsed := input.ContextWindow.TotalInputTokens + input.ContextWindow.TotalOutputTokens
		if totalUsed > 0 {
			usedPct = float64(totalUsed) / float64(input.ContextWindow.ContextWindowSize) * 100.0
			if usedPct > 100 {
				usedPct = 100
			}
		}
	}
	pctInt := int(usedPct)

	// Token string formatting
	ctxUsed := input.ContextWindow.TotalInputTokens + input.ContextWindow.TotalOutputTokens
	inputTokFmt := humanFormat(input.ContextWindow.TotalInputTokens)
	outputTokFmt := humanFormat(input.ContextWindow.TotalOutputTokens)
	ctxLimitFmt := humanFormat(input.ContextWindow.ContextWindowSize)
	ctxUsedFmt := humanFormat(ctxUsed)

	turnInputTokens := 0
	turnOutputTokens := 0
	if input.ContextWindow.CurrentUsage != nil {
		turnInputTokens = input.ContextWindow.CurrentUsage.InputTokens
		turnOutputTokens = input.ContextWindow.CurrentUsage.OutputTokens
	}
	turnInputFmt := humanFormat(turnInputTokens)
	turnOutputFmt := humanFormat(turnOutputTokens)

	cliVerFmt := ""
	if options["showVersion"] && input.Version != "" {
		cliVerFmt = theme.dotL1 + FG_GRAY + "v" + input.Version + "/sl-" + getStatuslineVersion() + R
		if updateAvailable != "" {
			if useClassicIcons {
				cliVerFmt += FG_BRIGHT_YELLOW + " [update " + updateAvailable + "]" + R
			} else {
				cliVerFmt += FG_BRIGHT_YELLOW + " 󰚔 " + updateAvailable + R
			}
		}
	}

	userFmt := ""
	if options["showUser"] && (input.PlanTier != "" || input.Email != "") {
		userInfo := ""
		if input.PlanTier != "" && input.Email != "" {
			userInfo = input.PlanTier + " (" + input.Email + ")"
		} else if input.PlanTier != "" {
			userInfo = input.PlanTier
		} else {
			userInfo = input.Email
		}
		// Truncate user info if too long
		if len(userInfo) > 35 {
			userInfo = userInfo[:32] + "..."
		}
		if useClassicIcons {
			userFmt = theme.dotL1 + FG_GRAY + userInfo + R
		} else {
			userFmt = theme.dotL1 + FG_GRAY + "󰇮 " + userInfo + R
		}
	}

	// Host details
	hostName, _ := os.Hostname()
	tsIP := getTailscaleIP()
	hostFmt := ""
	if options["showHost"] && hostName != "" {
		hostDetails := hostName
		if tsIP != "" {
			hostDetails = hostName + " (" + tsIP + ")"
		}
		if useClassicIcons {
			hostFmt = theme.dotL1 + FG_BRIGHT_BLUE + hostDetails + R
		} else {
			hostFmt = theme.dotL1 + FG_BRIGHT_BLUE + "󰒋 " + hostDetails + R
		}
	}

	// Power Status
	powerFmt := ""
	if options["showPower"] {
		hasBattery, batteryPct, acOnline := getPowerStatus()
		if hasBattery || !acOnline {
			// Running on battery/UPS
			if useClassicIcons {
				if batteryPct > 0 {
					powerFmt = FG_BRIGHT_YELLOW + theme.iconBat + ":" + strconv.Itoa(batteryPct) + "%" + R
				} else {
					powerFmt = FG_BRIGHT_YELLOW + theme.iconBat + R
				}
			} else {
				if batteryPct > 0 {
					powerFmt = FG_BRIGHT_YELLOW + theme.iconBat + " " + strconv.Itoa(batteryPct) + "%" + R
				} else {
					powerFmt = FG_BRIGHT_YELLOW + theme.iconBat + R
				}
			}
		} else {
			// Running on AC (Mains)
			if useClassicIcons {
				powerFmt = FG_GREEN + theme.iconAC + R
			} else {
				powerFmt = FG_GREEN + theme.iconAC + " AC" + R
			}
		}
	}

	// State Indicator
	var S string
	switch input.AgentState {
	case "idle":
		S = FG_BRIGHT_GREEN + B + " " + theme.iconReady + " READY" + R
	case "thinking":
		S = FG_BRIGHT_YELLOW + B + " " + theme.iconThinking + " THINKING" + R
	case "working":
		S = FG_BRIGHT_CYAN + B + " " + theme.iconWorking + " WORKING" + R
	case "tool_use":
		S = FG_BRIGHT_MAGENTA + B + " " + theme.iconTool + " TOOL" + R
	default:
		S = FG_WHITE + B + " " + theme.iconStateUnknown + " " + strings.ToUpper(input.AgentState) + R
	}



	// Model
	modelDisp := input.Model.DisplayName
	if modelDisp == "" {
		modelDisp = input.Model.ID
	}
	M := ""
	if options["showModel"] && modelDisp != "" {
		if useClassicIcons {
			M = theme.dotL1 + FG_BRIGHT_MAGENTA + I + modelDisp + R
		} else {
			M = theme.dotL1 + FG_BRIGHT_MAGENTA + I + theme.iconModel + " " + modelDisp + R
		}
	}

	// Sandbox Badge
	sb := ""
	if options["showSandbox"] {
		if input.Sandbox.Enabled {
			if input.Sandbox.AllowNetwork {
				sb = FG_GREEN + theme.iconSandboxNet + " ON (net)" + R
			} else {
				sb = FG_GREEN + theme.iconSandboxNoNet + " ON (no-net)" + R
			}
		} else {
			if useClassicIcons {
				sb = FG_GRAY + "sandbox off" + R
			} else {
				sb = FG_RED + theme.iconSandboxOff + " OFF" + R
			}
		}
	}

	// Context Bar (without progress bar)
	var fillGlobalColor string
	if pctInt >= 90 {
		fillGlobalColor = FG_BRIGHT_RED
	} else if pctInt >= 60 {
		fillGlobalColor = FG_BRIGHT_YELLOW
	} else {
		fillGlobalColor = FG_YELLOW
	}

	ctxBar := ""
	if options["showTokens"] && (usedPct > 0 || ctxUsed > 0) {
		if useClassicIcons {
			ctxBar = FG_GRAY + "ctx " + fillGlobalColor + fmt.Sprintf("%.2f", usedPct) + "%" + R
		} else {
			ctxBar = FG_YELLOW + theme.iconContextBar + " " + fillGlobalColor + fmt.Sprintf("%.2f", usedPct) + "%" + R
		}
	}

	// Stats formatting
	artFmt := ""
	subFmt := ""
	bgFmt := ""
	if options["showStats"] {
		if useClassicIcons {
			artFmt = FG_GRAY + "artifacts " + numColor + strconv.Itoa(input.ArtifactCount) + R
			subFmt = FG_GRAY + "subagents " + numColor + strconv.Itoa(len(input.Subagents)) + R
			bgFmt = FG_GRAY + "tasks " + numColor + strconv.Itoa(input.TaskCount) + R
		} else {
			artFmt = FG_BLUE + theme.iconArtifacts + " " + numColor + strconv.Itoa(input.ArtifactCount) + R
			subFmt = FG_CYAN + theme.iconSubagents + " " + numColor + strconv.Itoa(len(input.Subagents)) + R
			bgFmt = FG_MAGENTA + theme.iconTasks + " " + numColor + strconv.Itoa(input.TaskCount) + R
		}
	}

	costFmt := ""
	if options["showCost"] {
		var costParts []string
		// Use real cost if provided by CLI; otherwise estimate from token counts.
		costUSD := input.Cost.TotalCostUSD
		estimated := false
		if costUSD == 0 && (input.ContextWindow.TotalInputTokens > 0 || input.ContextWindow.TotalOutputTokens > 0) {
			costUSD = estimateCostUSD(input.Model.ID, input.ContextWindow.TotalInputTokens, input.ContextWindow.TotalOutputTokens)
			estimated = true
		}
		if costUSD > 0 {
			prefix := ""
			if estimated {
				prefix = "~"
			}
			if useClassicIcons {
				costParts = append(costParts, fmt.Sprintf("%s$%.2f", prefix, costUSD))
			} else {
				costParts = append(costParts, fmt.Sprintf("󰠵 %s$%.2f", prefix, costUSD))
			}
		}
		// LOC diff: use CLI-provided value if available, otherwise read from git.
		linesAdded := input.Cost.TotalLinesAdded
		linesRemoved := input.Cost.TotalLinesRemoved
		if linesAdded == 0 && linesRemoved == 0 && input.CWD != "" {
			linesAdded, linesRemoved = getGitLOC(input.CWD)
		}
		if linesAdded > 0 || linesRemoved > 0 {
			costParts = append(costParts, fmt.Sprintf("+%d -%d", linesAdded, linesRemoved))
		}
		if len(costParts) > 0 {
			costFmt = FG_WHITE + strings.Join(costParts, " ") + R
		}
	}

	dirFmt := ""
	if options["showDir"] {
		dirFmt = formatDirectorySegment(theme, input.CWD, useClassicIcons)
	}

	convFmt := ""
	if input.ConversationID != "" {
		shortConv := input.ConversationID
		if len(shortConv) > 8 {
			shortConv = shortConv[:8]
		}
		if useClassicIcons {
			convFmt = theme.dotL1 + FG_GRAY + shortConv + R
		} else {
			convFmt = theme.dotL1 + FG_GRAY + theme.iconConv + " " + shortConv + R
		}
	}

	tokDetailsWide := ""
	tokDetailsMed := ""
	if ctxUsed > 0 {
		turnStr := ""
		if turnInputTokens > 0 || turnOutputTokens > 0 {
			turnStr = " | turn: +" + turnInputFmt + "/" + turnOutputFmt
		}
		if useClassicIcons {
			tokDetailsWide = " (" + ctxUsedFmt + "/" + ctxLimitFmt + ")" + theme.dotL2 + "(total: " + inputTokFmt + "/" + outputTokFmt + turnStr + ")"
			tokDetailsMed = " (" + ctxUsedFmt + "/" + ctxLimitFmt + ")"
		} else {
			tokDetailsWide = " (" + ctxUsedFmt + "/" + ctxLimitFmt + ")" + theme.dotL2 + FG_YELLOW + theme.iconTokSum + " " + R + " (total: " + inputTokFmt + "/" + outputTokFmt + turnStr + ")"
			tokDetailsMed = " (" + ctxUsedFmt + "/" + ctxLimitFmt + ")"
		}
	}

	// ─── Quotas ──────────────────────────────────────────────────────────────
	var q5H, qWK float64 = -1, -1
	var q5HReset, qWKReset int = -1, -1

	// Determine which quota bucket to read based on the active model.
	// Gemini models → "gemini-5h" / "gemini-weekly"
	// All others (Claude, GPT, etc.) → "3p-5h" / "3p-weekly"
	isGeminiModel := strings.Contains(strings.ToLower(input.Model.ID), "gemini")

	if input.Quota != nil {
		var key5H, keyWK string
		if isGeminiModel {
			key5H = "gemini-5h"
			keyWK = "gemini-weekly"
		} else {
			key5H = "3p-5h"
			keyWK = "3p-weekly"
		}

		if q, ok := input.Quota[key5H]; ok && q != nil && q.RemainingFraction != nil {
			q5H = *q.RemainingFraction * 100.0
		}
		if q, ok := input.Quota[keyWK]; ok && q != nil && q.RemainingFraction != nil {
			qWK = *q.RemainingFraction * 100.0
		}
		if q, ok := input.Quota[key5H]; ok && q != nil && q.ResetInSeconds != nil {
			q5HReset = *q.ResetInSeconds
		}
		if q, ok := input.Quota[keyWK]; ok && q != nil && q.ResetInSeconds != nil {
			qWKReset = *q.ResetInSeconds
		}
	}


	// ─── Output Assembly ─────────────────────────────────────────────────────
	if input.TerminalWidth >= 180 {
		// Gather line 1 parts
		line1Parts := []string{S, cliVerFmt, userFmt, hostFmt, M, dirFmt, convFmt}

		// Gather line 2 parts
		var line2Parts []string
		if artFmt != "" {
			line2Parts = append(line2Parts, artFmt)
		}
		if subFmt != "" {
			line2Parts = append(line2Parts, subFmt)
		}
		if bgFmt != "" {
			line2Parts = append(line2Parts, bgFmt)
		}
		if costFmt != "" {
			line2Parts = append(line2Parts, costFmt)
		}
		if sb != "" {
			line2Parts = append(line2Parts, sb)
		}
		if ctxBar != "" {
			line2Parts = append(line2Parts, ctxBar+tokDetailsWide)
		}
		if q5H >= 0 {
			if q5HBar := makeQuotaBar(theme, q5H, "5H", FG_BRIGHT_CYAN, q5HReset); q5HBar != "" {
				line2Parts = append(line2Parts, q5HBar)
			}
		}
		if qWK >= 0 {
			if qWKBar := makeQuotaBar(theme, qWK, "7D", FG_BRIGHT_MAGENTA, qWKReset); qWKBar != "" {
				line2Parts = append(line2Parts, qWKBar)
			}
		}
		if powerFmt != "" {
			line2Parts = append(line2Parts, powerFmt)
		}

		line1 := joinAndWrap(theme, line1Parts, "", input.TerminalWidth, "  ")
		line2 := joinAndWrap(theme, line2Parts, theme.dotL2, input.TerminalWidth, "  ")

		if !strings.Contains(line1, "\n") && !strings.Contains(line2, "\n") && visibleLen(line1)+visibleLen(line2)+2 <= input.TerminalWidth {
			fmt.Println(printRightAligned(line1, line2, input.TerminalWidth))
		} else {
			fmt.Println(line1)
			for _, l2 := range strings.Split(line2, "\n") {
				fmt.Println(printRightAligned("", l2, input.TerminalWidth))
			}
		}

	} else if input.TerminalWidth >= 90 {
		line1Parts := []string{S, cliVerFmt, userFmt, hostFmt, M, dirFmt}

		var line2Parts []string
		if ctxBar != "" {
			line2Parts = append(line2Parts, ctxBar+tokDetailsMed)
		}
		if artFmt != "" {
			line2Parts = append(line2Parts, artFmt)
		}
		if subFmt != "" {
			line2Parts = append(line2Parts, subFmt)
		}
		if bgFmt != "" {
			line2Parts = append(line2Parts, bgFmt)
		}
		if costFmt != "" {
			line2Parts = append(line2Parts, costFmt)
		}
		if sb != "" {
			line2Parts = append(line2Parts, sb)
		}
		if q5H >= 0 {
			if q5HBar := makeQuotaBar(theme, q5H, "5H", FG_BRIGHT_CYAN, q5HReset); q5HBar != "" {
				line2Parts = append(line2Parts, q5HBar)
			}
		}
		if qWK >= 0 {
			if qWKBar := makeQuotaBar(theme, qWK, "7D", FG_BRIGHT_MAGENTA, qWKReset); qWKBar != "" {
				line2Parts = append(line2Parts, qWKBar)
			}
		}
		if powerFmt != "" {
			line2Parts = append(line2Parts, powerFmt)
		}

		contentWidth := input.TerminalWidth - 2
		if contentWidth < 40 {
			contentWidth = 40
		}

		line1 := joinAndWrap(theme, line1Parts, "", contentWidth, "  ")
		line2 := joinAndWrap(theme, line2Parts, theme.dotL2, contentWidth, "  ")

		line1Lines := strings.Split(line1, "\n")
		fmt.Println(FG_GRAY + "╭─" + R + line1Lines[0])
		for i := 1; i < len(line1Lines); i++ {
			fmt.Println(FG_GRAY + "│ " + R + line1Lines[i])
		}

		line2Lines := strings.Split(line2, "\n")
		for i := 0; i < len(line2Lines)-1; i++ {
			fmt.Println(FG_GRAY + "│ " + R + line2Lines[i])
		}
		fmt.Println(FG_GRAY + "╰─" + R + line2Lines[len(line2Lines)-1])

	} else {
		mShort := ""
		if modelDisp != "" {
			limit := 12
			if len(modelDisp) < limit {
				limit = len(modelDisp)
			}
			if useClassicIcons {
				mShort = FG_GRAY + " ╱ " + FG_BRIGHT_MAGENTA + modelDisp[:limit] + R
			} else {
				mShort = FG_GRAY + " ╱ " + FG_BRIGHT_MAGENTA + theme.iconModel + " " + modelDisp[:limit] + R
			}
		}

		fmt.Println(S + mShort)

		var line2Parts []string
		if ctxBar != "" {
			line2Parts = append(line2Parts, ctxBar)
		}
		if bgFmt != "" {
			line2Parts = append(line2Parts, bgFmt)
		}
		if costFmt != "" {
			line2Parts = append(line2Parts, costFmt)
		}
		if q5H >= 0 {
			if q5HBar := makeQuotaBar(theme, q5H, "5H", FG_BRIGHT_CYAN, q5HReset); q5HBar != "" {
				line2Parts = append(line2Parts, q5HBar)
			}
		}
		if qWK >= 0 {
			if qWKBar := makeQuotaBar(theme, qWK, "7D", FG_BRIGHT_MAGENTA, qWKReset); qWKBar != "" {
				line2Parts = append(line2Parts, qWKBar)
			}
		}
		if powerFmt != "" {
			line2Parts = append(line2Parts, powerFmt)
		}

		line2 := joinAndWrap(theme, line2Parts, theme.dotL2, input.TerminalWidth, "  ")
		fmt.Println(line2)
	}
}
