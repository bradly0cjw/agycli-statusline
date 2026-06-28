# agycli-statusline

A Go statusline program for the **[Antigravity CLI (Community Fork)](https://github.com/weby-homelab/antigravity-cli)** (`agy`) that displays session telemetry in a responsive, Nerd Font-styled format with clickable OSC 8 hyperlinks.

<!--
󰟷 THINKING | v0.1.3/sl-487e9fc | 󰇮 Google AI Pro |  Gemini 2.0 Flash
╭─ 󰟷 THINKING | v0.1.3/sl-487e9fc |  Gemini 2.0 Flash
│    owner/repo  main*
│ 󱍏 42.5% | 󰠵 $0.05 +120 -15 | 󰴴 ON (no-net) | 5H 85.0% ⌛️ 1h
╰─  7D 92.0% ⌛️ 1d | 󰚥 AC
-->

Each line displays: **Agent state** · **CLI/Statusline version** · **Subscription Tier / Email** · **Active Model** · **CWD & Upstream Repository (OSC 8 Clickable Hyperlinks)** · **Developer stats** (Artifacts / Subagents / Tasks) · **Session Cost & LOC Diff** · **Sandbox state** · **Context token usage** · **Rate limit quotas** (5h/7d with approach countdowns) · **Power/AC/Battery capacity**.

Empty segments are omitted automatically. Layouts wrap cleanly based on terminal columns to avoid messy wrapping.

---

## 🛠️ Requirements

- Go 1.21+ (if compiling from source)
- A [Nerd Font](https://www.nerdfonts.com/) in your terminal
- A terminal with OSC 8 hyperlink support (Ghostty, iTerm2, WezTerm, Kitty)

---

## 📥 Install

### Go

```sh
go install github.com/bradly0cjw/agycli-statusline@latest
```

### Linux & macOS (One-liner script)

```sh
curl -fsSL https://raw.githubusercontent.com/bradly0cjw/agycli-statusline/main/install.sh | bash
```

### Windows (PowerShell One-liner script)

```powershell
irm https://raw.githubusercontent.com/bradly0cjw/agycli-statusline/main/install.ps1 | iex
```

---

## ⚙️ Configure

Add to your `~/.gemini/antigravity-cli/settings.json` (or `%USERPROFILE%\.gemini\antigravity-cli\settings.json`):

```json
{
  "statusLine": {
    "type": "command",
    "command": "/home/user/.antigravity/statusline"
  }
}
```
*(Use `"antigravity-cli-statusline"` as the command if installed via Go tool)*

### Classic Mode (Terminals without Nerd Fonts)
Append `--classic` (or `--no-nerdfont` / `--compatibility`) to the command parameter to fallback to clean ASCII/unicode symbols.

---

## ⚙️ Settings TUI Menu

Customize which metrics to show or hide by launching the terminal settings selector:

```sh
# Script path
~/.antigravity/statusline --settings

# Go path
agycli-statusline --settings
```
This opens an interactive keyboard menu to toggle stats. To prevent your settings from being wiped when you retoggle `/statusline` in the `agy` CLI, all customizations are safely stored in a separate file at `~/.antigravity/config.json`.

---

## 🔄 Auto-Updates & Tracks

The statusline has a built-in background self-updating manager:
- **Daily checks**: Silent background checker runs once every 24 hours. A badge (`󰚔 v0.1.4` or `󰚔 487e9fc`) appears in the statusline when a new update is found.
- **Manual update**: Run with `--update` to download and replace the running binary.
- **Immediate check**: Force-refresh the update cache using `--refresh` or `-r`.
- **Tracks**:
  - `release` track (Default): Fetches prebuilt binaries from GitHub Releases.
  - `canary` track: Downloads the latest code from `main`, clones it into a temporary folder, compiles it locally, and deploys it atomically. Toggle your active track via Option 12 in the settings TUI.

---

## 🗑️ Uninstallation

Run the permanent uninstaller copy created in your folder:

### Linux / macOS
```bash
~/.antigravity/uninstall.sh
```

### Windows
```powershell
irm $HOME\.antigravity\uninstall.ps1 | iex
```

---

## 🤝 Acknowledgments

- **[cc-statusline](https://github.com/rileychh/cc-statusline)**: Inspired by and heavily referencing the layout schemas, OSC 8 branch and folder link strategies, and rate limit countdown designs.
- **[antigravity-cli](https://github.com/weby-homelab/antigravity-cli)**: The core terminal client.

---

<p align="center">
  Built by Bradly Chang<br>
  &copy; 2026 Bradly Chang
</p>
