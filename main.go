package main

import (
	"archive/tar"
	"bufio"
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

	"charm.land/bubbles/v2/filepicker"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/colorprofile"
	qrcode "github.com/skip2/go-qrcode"
)

type Attachment struct {
	ContentType string `json:"contentType"`
	Filename    string `json:"filename"`
	ID          string `json:"id"`
	Size        int    `json:"size"`
	Width       int    `json:"width"`
	Height      int    `json:"height"`
	Caption     string `json:"caption"`
}

type Config struct {
	Account string `json:"account"`
	Port    int    `json:"port"`
}

func configDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "signal-tui")
}

func configPath() string {
	return filepath.Join(configDir(), "config.json")
}

func historyPath() string {
	return filepath.Join(configDir(), "history.json")
}

type historyEntry struct {
	From string    `json:"from"`
	Text string    `json:"text"`
	Time time.Time `json:"time"`
	IsMe bool      `json:"isMe"`
}

func loadHistory() map[string][]Message {
	data, err := os.ReadFile(historyPath())
	if err != nil {
		return make(map[string][]Message)
	}
	var raw map[string][]historyEntry
	if err := json.Unmarshal(data, &raw); err != nil {
		return make(map[string][]Message)
	}
	result := make(map[string][]Message)
	for k, v := range raw {
		for _, e := range v {
			result[k] = append(result[k], Message{
				From:    e.From,
				Text:    e.Text,
				Time:    e.Time,
				IsMe:    e.IsMe,
				Contact: k,
			})
		}
	}
	return result
}

func saveHistory(history map[string][]Message) {
	raw := make(map[string][]historyEntry)
	for k, v := range history {
		for _, m := range v {
			raw[k] = append(raw[k], historyEntry{
				From: m.From,
				Text: m.Text,
				Time: m.Time,
				IsMe: m.IsMe,
			})
		}
	}
	data, err := json.MarshalIndent(raw, "", "  ")
	if err != nil {
		return
	}
	os.MkdirAll(configDir(), 0700)
	os.WriteFile(historyPath(), data, 0600)
}

func loadConfig() (*Config, error) {
	data, err := os.ReadFile(configPath())
	if err != nil {
		return nil, err
	}
	var c Config
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, err
	}
	return &c, nil
}

func saveConfig(c *Config) error {
	os.MkdirAll(configDir(), 0700)
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(configPath(), data, 0600)
}

type Contact struct {
	Number string `json:"number"`
	UUID   string `json:"uuid"`
	Name   string `json:"name"`
	Given  string `json:"givenName"`
}

func (c Contact) DisplayName() string {
	if c.Given != "" {
		return c.Given
	}
	if c.Name != "" {
		return c.Name
	}
	return c.Number
}

type rpcRequest struct {
	JSONRPC string      `json:"jsonrpc"`
	Method  string      `json:"method"`
	Params  interface{} `json:"params"`
	ID      int         `json:"id"`
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	Result  json.RawMessage `json:"result"`
	Error   *rpcError       `json:"error"`
	ID      int             `json:"id"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}
type Message struct {
	From        string
	Text        string
	Time        time.Time
	IsMe        bool
	Contact     string
	Sent        bool
	Receipt     int
	Reactions   map[string]int
	Attachments []Attachment
	LocalPaths  []string
}

const (
	stateMissingDep = iota
	stateInstalling
	stateSetup
	stateQRCode
	stateWaitingLink
	stateStartingDaemon
	stateChat
)

type model struct {
	state     int
	config    *Config
	port      int
	account   string
	rpcURL    string
	eventsURL string

	setupMsg string
	qrCode   string
	linkURL  string
	setupErr string

	contacts      []Contact
	selected      int
	messages      []Message
	history       map[string][]Message
	typing        string
	confirmDelete bool
	input         textinput.Model
	focus         int
	width         int
	height        int
	status        string

	showHelp          bool
	fpActive          bool
	fp                filepicker.Model
	fpHeight          int
	lastAttachment    string
	pendingAttachment string
	pendingAtt        Attachment

	theme        Theme
	themeName    string
	isDark       bool
	colorProfile colorprofile.Profile

	daemonCmd      *exec.Cmd
	beforeAccounts []string
}

var daemonCmd *exec.Cmd

func newModel() model {
	ti := textinput.New()
	ti.Placeholder = "Type a message..."
	ti.CharLimit = 4096
	ti.SetWidth(60)

	fp := filepicker.New()
	fp.CurrentDirectory = os.ExpandEnv("$HOME")
	fp.AllowedTypes = []string{".png", ".jpg", ".jpeg", ".gif", ".webp", ".heic", ".pdf", ".doc", ".docx", ".odt", ".txt", ".md", ".csv", ".xlsx", ".pptx"}
	fp.ShowHidden = false
	fp.DirAllowed = true
	fp.FileAllowed = true
	fp.AutoHeight = true

	cfg, _ := loadConfig()
	profile := colorprofile.Detect(os.Stdout, os.Environ())
	m := model{
		input:        ti,
		fp:           fp,
		status:       "Starting...",
		history:      make(map[string][]Message),
		theme:        NewTheme(profile, true),
		themeName:    "",
		isDark:       true,
		colorProfile: profile,
	}
	if _, err := exec.LookPath("signal-cli"); err != nil {
		m.state = stateMissingDep
		m.port = findAvailablePort(8081)
		return m
	}
	if cfg != nil && cfg.Account != "" {
		m.config = cfg
		m.account = cfg.Account
		m.port = cfg.Port
		if m.port == 0 {
			m.port = findAvailablePort(8081)
		}
		m.state = stateStartingDaemon
	} else {
		m.state = stateSetup
		m.port = findAvailablePort(8081)
	}
	return m
}

func findAvailablePort(start int) int {
	for p := start; p < start+100; p++ {
		ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", p))
		if err == nil {
			ln.Close()
			return p
		}
	}
	return start
}

func readAccountUUIDs() []string {
	var uuids []string
	paths := []string{
		os.ExpandEnv("$HOME/.local/share/signal-cli/data/accounts.json"),
		os.ExpandEnv("$HOME/.config/signal-cli/data/accounts.json"),
		"/root/.local/share/signal-cli/data/accounts.json",
	}
	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		var af struct {
			Accounts []struct {
				UUID string `json:"uuid"`
			} `json:"accounts"`
		}
		if err := json.Unmarshal(data, &af); err != nil {
			continue
		}
		for _, a := range af.Accounts {
			uuids = append(uuids, a.UUID)
		}
	}
	return uuids
}

func waitForLink(before []string) tea.Cmd {
	return func() tea.Msg {
		// Wait for the link process to exit (it exits after provisioning).
		done := make(chan error, 1)
		go func() {
			if linkProcess != nil {
				done <- linkProcess.Wait()
			} else {
				done <- fmt.Errorf("no link process")
			}
		}()
		select {
		case err := <-done:
			if err != nil {
				msg := err.Error()
				if linkStderr != "" {
					msg = msg + ": " + strings.TrimSpace(linkStderr)
				}
				linkProcess = nil
				return linkCompleteMsg{err: fmt.Errorf("link failed: %s", msg)}
			}
		case <-time.After(120 * time.Second):
			if linkProcess != nil {
				linkProcess.Process.Kill()
			}
			linkProcess = nil
			return linkCompleteMsg{err: fmt.Errorf("link timed out")}
		}
		linkProcess = nil
		// Now check accounts.json for a new UUID.
		after := readAccountUUIDs()
		for _, u := range after {
			found := false
			for _, b := range before {
				if u == b {
					found = true
					break
				}
			}
			if !found {
				return linkCompleteMsg{account: u}
			}
		}
		if len(after) > 0 {
			return linkCompleteMsg{account: after[len(after)-1]}
		}
		return linkCompleteMsg{err: fmt.Errorf("no account found after link")}
	}
}
func installSignalCLI() tea.Cmd {
	return func() tea.Msg {
		// Download latest signal-cli release tarball and extract to ~/.local/bin
		installDir := os.ExpandEnv("$HOME/.local/bin")
		if err := os.MkdirAll(installDir, 0700); err != nil {
			return signalCLIInstalledMsg{err: fmt.Errorf("mkdir: %v", err)}
		}
		// Get latest release info
		client := &http.Client{Timeout: 30 * time.Second}
		resp, err := client.Get("https://api.github.com/repos/AsamK/signal-cli/releases/latest")
		if err != nil {
			return signalCLIInstalledMsg{err: fmt.Errorf("fetch release info: %v", err)}
		}
		defer resp.Body.Close()
		var release struct {
			TagName string `json:"tag_name"`
			Assets  []struct {
				Name               string `json:"name"`
				BrowserDownloadURL string `json:"browser_download_url"`
			} `json:"assets"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
			return signalCLIInstalledMsg{err: fmt.Errorf("parse release: %v", err)}
		}
		var url string
		expected := "signal-cli-" + strings.TrimPrefix(release.TagName, "v") + ".tar.gz"
		for _, a := range release.Assets {
			if a.Name == expected {
				url = a.BrowserDownloadURL
				break
			}
		}
		if url == "" {
			// fallback: first tarball that isn't json-schemas or native/client
			for _, a := range release.Assets {
				if strings.HasSuffix(a.Name, ".tar.gz") && !strings.Contains(a.Name, "json-schemas") && !strings.Contains(a.Name, "-Linux-") {
					url = a.BrowserDownloadURL
					break
				}
			}
		}
		if url == "" {
			return signalCLIInstalledMsg{err: fmt.Errorf("no tarball in release %s", release.TagName)}
		}
		// Download tarball
		dlClient := &http.Client{Timeout: 300 * time.Second}
		resp2, err := dlClient.Get(url)
		if err != nil {
			return signalCLIInstalledMsg{err: fmt.Errorf("download: %v", err)}
		}
		defer resp2.Body.Close()
		tmp, err := os.CreateTemp("", "signal-cli-*.tar.gz")
		if err != nil {
			return signalCLIInstalledMsg{err: fmt.Errorf("temp: %v", err)}
		}
		tmpName := tmp.Name()
		defer os.Remove(tmpName)
		if _, err := io.Copy(tmp, resp2.Body); err != nil {
			tmp.Close()
			return signalCLIInstalledMsg{err: fmt.Errorf("write: %v", err)}
		}
		tmp.Close()
		// Extract
		f, err := os.Open(tmpName)
		if err != nil {
			return signalCLIInstalledMsg{err: fmt.Errorf("open: %v", err)}
		}
		defer f.Close()
		gz, err := gzip.NewReader(f)
		if err != nil {
			return signalCLIInstalledMsg{err: fmt.Errorf("gzip: %v", err)}
		}
		defer gz.Close()
		tr := tar.NewReader(gz)
		for {
			hdr, err := tr.Next()
			if err == io.EOF {
				break
			}
			if err != nil {
				return signalCLIInstalledMsg{err: fmt.Errorf("tar: %v", err)}
			}
			if hdr.Typeflag != tar.TypeReg {
				continue
			}
			base := filepath.Base(hdr.Name)
			if base != "signal-cli" {
				continue
			}
			outPath := filepath.Join(installDir, "signal-cli")
			out, err := os.OpenFile(outPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0755)
			if err != nil {
				return signalCLIInstalledMsg{err: fmt.Errorf("create binary: %v", err)}
			}
			if _, err := io.Copy(out, tr); err != nil {
				out.Close()
				return signalCLIInstalledMsg{err: fmt.Errorf("extract: %v", err)}
			}
			out.Close()
		}
		// Ensure PATH includes installDir for this process
		if !strings.Contains(os.Getenv("PATH"), installDir) {
			os.Setenv("PATH", installDir+":"+os.Getenv("PATH"))
		}
		return signalCLIInstalledMsg{err: nil}
	}
}
func runLink() tea.Cmd {
	return func() tea.Msg {
		linkProcess = exec.Command("signal-cli", "link", "-n", "TUI")
		stdout, err := linkProcess.StdoutPipe()
		if err != nil {
			return linkCompleteMsg{err: fmt.Errorf("failed to start link: %v", err)}
		}
		stderr, err := linkProcess.StderrPipe()
		if err != nil {
			return linkCompleteMsg{err: fmt.Errorf("failed to start link: %v", err)}
		}
		linkStderr = ""
		if err := linkProcess.Start(); err != nil {
			return linkCompleteMsg{err: fmt.Errorf("failed to start link: %v", err)}
		}
		scanner := bufio.NewScanner(stdout)
		scanner.Scan()
		url := strings.TrimSpace(scanner.Text())
		if url == "" {
			b, _ := io.ReadAll(stderr)
			linkStderr = string(b)
			linkProcess.Process.Kill()
			linkProcess = nil
			return linkCompleteMsg{err: fmt.Errorf("failed to get link URL: %s", strings.TrimSpace(linkStderr))}
		}
		go func() {
			for scanner.Scan() {
			}
		}()
		go func() {
			b, _ := io.ReadAll(stderr)
			linkStderr = string(b)
		}()
		qr, err := qrcode.New(url, qrcode.Medium)
		if err != nil {
			return linkCompleteMsg{err: fmt.Errorf("qr generation failed: %v", err)}
		}
		return qrCodeMsg{url: url, qr: qr.ToSmallString(false)}
	}
}

var linkProcess *exec.Cmd
var linkStderr string

type linkCompleteMsg struct {
	account string
	err     error
}

type signalCLIInstalledMsg struct {
	err error
}

type daemonReadyMsg struct {
	port int
	err  error
}

type qrCodeMsg struct {
	url string
	qr  string
}

type pollAccountMsg struct {
	account string
}

func startDaemon(account string, port int) tea.Cmd {
	return func() tea.Msg {
		cmd := exec.Command("signal-cli", "-a", account, "daemon", "--http", fmt.Sprintf("127.0.0.1:%d", port), "--receive-mode", "on-connection")
		var stderr bytes.Buffer
		cmd.Stdout = nil
		cmd.Stderr = &stderr
		if err := cmd.Start(); err != nil {
			return daemonReadyMsg{port: port, err: fmt.Errorf("failed to start daemon: %v", err)}
		}
		daemonCmd = cmd
		for i := 0; i < 30; i++ {
			time.Sleep(1 * time.Second)
			conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 500*time.Millisecond)
			if err == nil {
				conn.Close()
				return daemonReadyMsg{port: port}
			}
		}
		// Kill the failed daemon.
		if cmd.Process != nil {
			cmd.Process.Kill()
		}
		errMsg := strings.TrimSpace(stderr.String())
		if errMsg == "" {
			errMsg = "daemon did not start in time"
		}
		return daemonReadyMsg{port: port, err: fmt.Errorf("daemon did not start in time: %s", errMsg)}
	}
}

func loadContacts(port int, account string) tea.Cmd {
	return func() tea.Msg {
		req := rpcRequest{JSONRPC: "2.0", Method: "listContacts", Params: map[string]interface{}{"account": account}, ID: 1}
		body, _ := json.Marshal(req)
		url := fmt.Sprintf("http://127.0.0.1:%d/api/v1/rpc", port)
		resp, err := http.Post(url, "application/json", bytes.NewReader(body))
		if err != nil {
			return contactsLoadedMsg{err: err}
		}
		defer resp.Body.Close()
		data, _ := io.ReadAll(resp.Body)
		var rpcResp rpcResponse
		if err := json.Unmarshal(data, &rpcResp); err != nil {
			return contactsLoadedMsg{err: err}
		}
		if rpcResp.Error != nil {
			return contactsLoadedMsg{err: fmt.Errorf("%s", rpcResp.Error.Message)}
		}
		var contacts []Contact
		if err := json.Unmarshal(rpcResp.Result, &contacts); err != nil {
			var wrap struct {
				Contacts []Contact `json:"contacts"`
			}
			if err2 := json.Unmarshal(rpcResp.Result, &wrap); err2 == nil {
				contacts = wrap.Contacts
			}
		}
		return contactsLoadedMsg{contacts: contacts}
	}
}

type contactsLoadedMsg struct {
	contacts []Contact
	err      error
}

func sendMessage(port int, recipient string, text string) tea.Cmd {
	return sendMessageWithAttachment(port, recipient, text, "")
}

func sendMessageWithAttachment(port int, recipient string, text string, attachment string) tea.Cmd {
	return func() tea.Msg {
		params := map[string]interface{}{"recipient": recipient}
		if text != "" {
			params["message"] = text
		}
		if attachment != "" {
			params["attachments"] = []string{attachment}
		}
		req := rpcRequest{JSONRPC: "2.0", Method: "send", Params: params, ID: 2}
		body, _ := json.Marshal(req)
		url := fmt.Sprintf("http://127.0.0.1:%d/api/v1/rpc", port)
		resp, err := http.Post(url, "application/json", bytes.NewReader(body))
		if err != nil {
			return sendResultMsg{err: err}
		}
		defer resp.Body.Close()
		data, _ := io.ReadAll(resp.Body)
		var rpcResp rpcResponse
		if err := json.Unmarshal(data, &rpcResp); err != nil {
			return sendResultMsg{err: err}
		}
		if rpcResp.Error != nil {
			return sendResultMsg{err: fmt.Errorf("%s", rpcResp.Error.Message)}
		}
		var result struct {
			Timestamp int64 `json:"timestamp"`
		}
		json.Unmarshal(rpcResp.Result, &result)
		return sendResultMsg{timestamp: result.Timestamp}
	}
}

type sendResultMsg struct {
	timestamp int64
	err       error
}

func sendAttachment(port int, recipient string, path string) tea.Cmd {
	return sendMessageWithAttachment(port, recipient, "", path)
}

type typingMsg struct {
	from   string
	action string
}

type receiptMsg struct {
	source     string
	isDelivery bool
	isRead     bool
	isViewed   bool
	timestamps []int64
}

type reactionMsg struct {
	source          string
	emoji           string
	targetTimestamp int64
	isRemove        bool
}

type messageReceivedMsg struct {
	msg Message
}

type attachmentReadyMsg struct {
	contact string
	time    time.Time
	att     Attachment
	path    string
	err     error
}

func listenEvents(port int, account string) tea.Cmd {
	return func() tea.Msg {
		url := fmt.Sprintf("http://127.0.0.1:%d/api/v1/events?account=%s", port, account)
		resp, err := http.Get(url)
		if err != nil {
			time.Sleep(time.Second)
			return listenEvents(port, account)()
		}
		defer resp.Body.Close()
		scanner := bufio.NewScanner(resp.Body)
		scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
		for scanner.Scan() {
			line := scanner.Text()
			if !strings.HasPrefix(line, "data:") {
				continue
			}
			data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			var ev struct {
				Envelope struct {
					Source      string `json:"source"`
					SourceName  string `json:"sourceName"`
					Timestamp   int64  `json:"timestamp"`
					DataMessage *struct {
						Message     string       `json:"message"`
						Attachments []Attachment `json:"attachments"`
						Reaction    *struct {
							Emoji               string `json:"emoji"`
							TargetAuthor        string `json:"targetAuthor"`
							TargetSentTimestamp int64  `json:"targetSentTimestamp"`
							IsRemove            bool   `json:"isRemove"`
						} `json:"reaction"`
					} `json:"dataMessage"`
					TypingMessage *struct {
						Action string `json:"action"`
					} `json:"typingMessage"`
					ReceiptMessage *struct {
						When       int64   `json:"when"`
						IsDelivery bool    `json:"isDelivery"`
						IsRead     bool    `json:"isRead"`
						IsViewed   bool    `json:"isViewed"`
						Timestamps []int64 `json:"timestamps"`
					} `json:"receiptMessage"`
				} `json:"envelope"`
			}
			if err := json.Unmarshal([]byte(data), &ev); err != nil {
				continue
			}
			env := ev.Envelope
			if env.DataMessage != nil {
				if env.DataMessage.Reaction != nil {
					r := env.DataMessage.Reaction
					from := env.SourceName
					if from == "" {
						from = env.Source
					}
					return reactionMsg{source: from, emoji: r.Emoji, targetTimestamp: r.TargetSentTimestamp, isRemove: r.IsRemove}
				}
				msg := Message{
					From:        env.SourceName,
					Text:        env.DataMessage.Message,
					Time:        time.UnixMilli(env.Timestamp),
					IsMe:        false,
					Contact:     env.Source,
					Attachments: env.DataMessage.Attachments,
				}
				if msg.From == "" {
					msg.From = env.Source
				}
				return messageReceivedMsg{msg: msg}
			}
			if env.TypingMessage != nil {
				from := env.SourceName
				if from == "" {
					from = env.Source
				}
				return typingMsg{from: from, action: env.TypingMessage.Action}
			}
			if env.ReceiptMessage != nil {
				return receiptMsg{source: env.Source, isDelivery: env.ReceiptMessage.IsDelivery, isRead: env.ReceiptMessage.IsRead, isViewed: env.ReceiptMessage.IsViewed, timestamps: env.ReceiptMessage.Timestamps}
			}
		}
		if err := scanner.Err(); err != nil {
			time.Sleep(time.Second)
		}
		return listenEvents(port, account)()
	}
}

func sendTypingIndicatorCmd(port int, recipient string) tea.Cmd {
	return func() tea.Msg {
		req := rpcRequest{JSONRPC: "2.0", Method: "sendTyping", Params: map[string]interface{}{"recipient": recipient}, ID: 99}
		body, _ := json.Marshal(req)
		url := fmt.Sprintf("http://127.0.0.1:%d/api/v1/rpc", port)
		http.Post(url, "application/json", bytes.NewReader(body))
		return nil
	}
}

func sendReadReceiptCmd(port int, account string, recipient string, ts int64) tea.Cmd {
	return func() tea.Msg {
		req := rpcRequest{JSONRPC: "2.0", Method: "sendReceipt", Params: map[string]interface{}{
			"account":          account,
			"recipient":        recipient,
			"targetTimestamps": []int64{ts},
			"type":             "read",
		}, ID: 5}
		body, _ := json.Marshal(req)
		url := fmt.Sprintf("http://127.0.0.1:%d/api/v1/rpc", port)
		resp, err := http.Post(url, "application/json", bytes.NewReader(body))
		if err != nil {
			return nil
		}
		resp.Body.Close()
		return nil
	}
}

func sendReactionCmd(port int, account string, recipient string, emoji string, targetAuthor string, targetTs int64, remove bool) tea.Cmd {
	return func() tea.Msg {
		req := rpcRequest{JSONRPC: "2.0", Method: "sendReaction", Params: map[string]interface{}{
			"account":         account,
			"recipient":       recipient,
			"emoji":           emoji,
			"targetAuthor":    targetAuthor,
			"targetTimestamp": targetTs,
			"remove":          remove,
		}, ID: 6}
		body, _ := json.Marshal(req)
		url := fmt.Sprintf("http://127.0.0.1:%d/api/v1/rpc", port)
		resp, err := http.Post(url, "application/json", bytes.NewReader(body))
		if err != nil {
			return nil
		}
		resp.Body.Close()
		return nil
	}
}

func cacheDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".cache", "signal-tui")
}

func attachmentCacheDir() string {
	return filepath.Join(cacheDir(), "attachments")
}

var allowedExtensions = map[string]bool{
	".png": true, ".jpg": true, ".jpeg": true, ".gif": true, ".webp": true, ".heic": true,
	".pdf": true, ".doc": true, ".docx": true, ".odt": true, ".txt": true, ".md": true,
	".csv": true, ".xlsx": true, ".pptx": true,
}

func isAllowedExtension(name string) bool {
	return allowedExtensions[strings.ToLower(filepath.Ext(name))]
}

func isSupportedAttachment(att Attachment) bool {
	ext := strings.ToLower(filepath.Ext(att.Filename))
	return map[string]bool{
		".png": true, ".jpg": true, ".jpeg": true, ".gif": true, ".webp": true, ".heic": true,
		".pdf": true, ".txt": true, ".md": true,
	}[ext]
}

func fetchAttachmentCmd(port int, account string, contact string, t time.Time, att Attachment) tea.Cmd {
	return func() tea.Msg {
		req := rpcRequest{JSONRPC: "2.0", Method: "getAttachment", Params: map[string]interface{}{"account": account, "id": att.ID}, ID: 3}
		body, _ := json.Marshal(req)
		url := fmt.Sprintf("http://127.0.0.1:%d/api/v1/rpc", port)
		resp, err := http.Post(url, "application/json", bytes.NewReader(body))
		if err != nil {
			return attachmentReadyMsg{contact: contact, time: t, att: att, err: err}
		}
		defer resp.Body.Close()
		data, _ := io.ReadAll(resp.Body)
		var rpcResp rpcResponse
		if err := json.Unmarshal(data, &rpcResp); err != nil {
			return attachmentReadyMsg{contact: contact, time: t, att: att, err: err}
		}
		if rpcResp.Error != nil {
			return attachmentReadyMsg{contact: contact, time: t, att: att, err: fmt.Errorf("%s", rpcResp.Error.Message)}
		}
		var result struct {
			Data string `json:"data"`
		}
		if err := json.Unmarshal(rpcResp.Result, &result); err != nil {
			result.Data = string(rpcResp.Result)
		}
		raw, err := base64.StdEncoding.DecodeString(strings.Trim(result.Data, "\""))
		if err != nil {
			return attachmentReadyMsg{contact: contact, time: t, att: att, err: err}
		}
		os.MkdirAll(attachmentCacheDir(), 0700)
		ext := filepath.Ext(att.Filename)
		if ext == "" {
			ext = ".bin"
		}
		base := filepath.Base(att.ID)
		if e := filepath.Ext(base); e != "" {
			base = base[:len(base)-len(e)]
		}
		path := filepath.Join(attachmentCacheDir(), base+ext)
		if err := os.WriteFile(path, raw, 0600); err != nil {
			return attachmentReadyMsg{contact: contact, time: t, att: att, err: err}
		}
		return attachmentReadyMsg{contact: contact, time: t, att: att, path: path}
	}
}

func saveAttachmentCmd(src string, att Attachment) tea.Cmd {
	return func() tea.Msg {
		destDir := filepath.Join(os.ExpandEnv("$HOME"), "Downloads", "signal-tui")
		os.MkdirAll(destDir, 0700)
		name := att.Filename
		if name == "" {
			name = filepath.Base(src)
		}
		dest := filepath.Join(destDir, name)
		if _, err := os.Stat(dest); err == nil {
			ext := filepath.Ext(name)
			base := name[:len(name)-len(ext)]
			dest = filepath.Join(destDir, fmt.Sprintf("%s_%d%s", base, time.Now().Unix(), ext))
		}
		data, err := os.ReadFile(src)
		if err != nil {
			return saveResultMsg{err: err}
		}
		if err := os.WriteFile(dest, data, 0600); err != nil {
			return saveResultMsg{err: err}
		}
		return saveResultMsg{path: dest}
	}
}

type saveResultMsg struct {
	path string
	err  error
}

func debugLog(format string, args ...interface{}) {
	f, err := os.OpenFile(filepath.Join(cacheDir(), "debug.log"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return
	}
	defer f.Close()
	fmt.Fprintf(f, time.Now().Format("15:04:05")+" "+format+"\n", args...)
}

func (m model) Init() tea.Cmd {
	cmds := []tea.Cmd{tea.RequestBackgroundColor}
	if m.state == stateStartingDaemon {
		cmds = append(cmds, startDaemon(m.account, m.port))
	}
	return tea.Batch(cmds...)
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if m.pendingAttachment != "" {
		if kp, ok := msg.(tea.KeyPressMsg); ok {
			path := m.pendingAttachment
			att := m.pendingAtt
			switch kp.String() {
			case "o":
				if _, err := exec.LookPath("xdg-open"); err != nil {
					m.pendingAttachment = ""
					m.status = "No file handler; saving instead"
					return m, saveAttachmentCmd(path, att)
				}
				if isSupportedAttachment(att) {
					m.pendingAttachment = ""
					cmd := exec.Command("xdg-open", path)
					return m, tea.ExecProcess(cmd, func(err error) tea.Msg {
						if err != nil {
							return sendResultMsg{err: err}
						}
						return nil
					})
				}
				m.status = "File not supported; use 's' to save"
				return m, nil
			case "s":
				m.pendingAttachment = ""
				return m, saveAttachmentCmd(path, att)
			case "d", "esc":
				m.pendingAttachment = ""
				return m, nil
			}
		}
		return m, nil
	}

	if m.fpActive {
		if ws, ok := msg.(tea.WindowSizeMsg); ok {
			m.width = ws.Width
			m.height = ws.Height
			h := m.height * 3 / 10
			if h < 5 {
				h = 5
			}
			m.fp.SetHeight(h)
			m.fpHeight = h
			fp, cmd := m.fp.Update(msg)
			m.fp = fp
			return m, cmd
		}
		if kp, ok := msg.(tea.KeyPressMsg); ok && kp.String() == "esc" {
			m.fpActive = false
			return m, nil
		}
		fp, cmd := m.fp.Update(msg)
		m.fp = fp
		if ok2, path := m.fp.DidSelectFile(msg); ok2 && path != "" {
			m.fpActive = false
			m.status = ""
			if m.selected < len(m.contacts) {
				contact := m.contacts[m.selected]
				recipient := contact.UUID
				if recipient == "" {
					recipient = contact.Number
				}
				msg2 := Message{From: "You", Text: "[attachment: " + filepath.Base(path) + "]", Time: time.Now(), IsMe: true, Contact: contact.Number}
				m.messages = append(m.messages, msg2)
				if m.history == nil {
					m.history = make(map[string][]Message)
				}
				m.history[contact.Number] = append(m.history[contact.Number], msg2)
				saveHistory(m.history)
				return m, tea.Batch(cmd, sendMessageWithAttachment(m.port, recipient, "", path))
			}
			return m, cmd
		}
		if ok2, _ := m.fp.DidSelectDisabledFile(msg); ok2 {
			if isAllowedExtension(m.fp.HighlightedPath()) {
				m.status = ""
			} else {
				m.status = "File type not allowed"
			}
			return m, cmd
		}
		return m, cmd
	}

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		w := m.width*3/4 - 6
		if w < 10 {
			w = 10
		}
		m.input.SetWidth(w)
		h := m.height * 3 / 10
		if h < 5 {
			h = 5
		}
		m.fp.SetHeight(h)
		m.fpHeight = h
		return m, nil

	case tea.ColorProfileMsg:
		m.colorProfile = msg.Profile
		m.theme = NewTheme(msg.Profile, m.isDark)
		return m, nil
	case tea.BackgroundColorMsg:
		m.isDark = msg.IsDark()
		m.theme = NewTheme(m.colorProfile, m.isDark)
		return m, nil

	case daemonReadyMsg:
		if msg.err != nil {
			m.setupErr = msg.err.Error()
			m.state = stateSetup
			return m, nil
		}
		m.port = msg.port
		if m.config == nil {
			m.config = &Config{}
		}
		m.config.Account = m.account
		m.config.Port = m.port
		saveConfig(m.config)
		m.rpcURL = fmt.Sprintf("http://127.0.0.1:%d/api/v1/rpc", m.port)
		m.eventsURL = fmt.Sprintf("http://127.0.0.1:%d/api/v1/events?account=%s", m.port, m.account)
		m.state = stateChat
		m.status = "Connected"
		return m, tea.Batch(loadContacts(m.port, m.account), listenEvents(m.port, m.account))

	case contactsLoadedMsg:
		debugLog("contactsLoadedMsg count=%d err=%v", len(msg.contacts), msg.err)
		if msg.err != nil {
			m.status = "Error: " + msg.err.Error()
			return m, nil
		}
		m.contacts = msg.contacts
		m.history = loadHistory()
		if len(m.contacts) > 0 {
			c := m.contacts[0]
			if msgs, ok := m.history[c.Number]; ok {
				m.messages = msgs
			}
		}
		m.status = fmt.Sprintf("%d contacts", len(m.contacts))
		return m, nil

	case receiptMsg:
		debugLog("receiptReceivedMsg src=%s delivery=%v read=%v viewed=%v ts=%v", msg.source, msg.isDelivery, msg.isRead, msg.isViewed, msg.timestamps)
		for i := range m.messages {
			if m.messages[i].IsMe {
				for _, ts := range msg.timestamps {
					if m.messages[i].Time.UnixMilli() == ts {
						if msg.isRead || msg.isViewed {
							m.messages[i].Receipt = 2
						} else if msg.isDelivery {
							if m.messages[i].Receipt < 1 {
								m.messages[i].Receipt = 1
							}
						}
					}
				}
			}
		}
		for c := range m.history {
			for i := range m.history[c] {
				if m.history[c][i].IsMe {
					for _, ts := range msg.timestamps {
						if m.history[c][i].Time.UnixMilli() == ts {
							if msg.isRead || msg.isViewed {
								m.history[c][i].Receipt = 2
							} else if msg.isDelivery {
								if m.history[c][i].Receipt < 1 {
									m.history[c][i].Receipt = 1
								}
							}
						}
					}
				}
			}
		}
		saveHistory(m.history)
		return m, listenEvents(m.port, m.account)

	case reactionMsg:
		debugLog("reactionReceivedMsg src=%s emoji=%s ts=%d remove=%v", msg.source, msg.emoji, msg.targetTimestamp, msg.isRemove)
		if msg.emoji == "✅" {
			return m, listenEvents(m.port, m.account)
		}
		for i := range m.messages {
			if m.messages[i].Time.UnixMilli() == msg.targetTimestamp {
				if m.messages[i].Reactions == nil {
					m.messages[i].Reactions = make(map[string]int)
				}
				if msg.isRemove {
					delete(m.messages[i].Reactions, msg.emoji)
				} else {
					m.messages[i].Reactions[msg.emoji]++
				}
			}
		}
		for c := range m.history {
			for i := range m.history[c] {
				if m.history[c][i].Time.UnixMilli() == msg.targetTimestamp {
					if m.history[c][i].Reactions == nil {
						m.history[c][i].Reactions = make(map[string]int)
					}
					if msg.isRemove {
						delete(m.history[c][i].Reactions, msg.emoji)
					} else {
						m.history[c][i].Reactions[msg.emoji]++
					}
				}
			}
		}
		saveHistory(m.history)
		return m, listenEvents(m.port, m.account)

	case sendResultMsg:
		if msg.err != nil {
			m.status = "Send failed: " + msg.err.Error()
			return m, nil
		}
		if msg.timestamp != 0 {
			// update last sent message
			for i := len(m.messages) - 1; i >= 0; i-- {
				if m.messages[i].IsMe {
					m.messages[i].Time = time.UnixMilli(msg.timestamp)
					m.messages[i].Sent = true
					break
				}
			}
			for c := range m.history {
				for i := len(m.history[c]) - 1; i >= 0; i-- {
					if m.history[c][i].IsMe {
						m.history[c][i].Time = time.UnixMilli(msg.timestamp)
						m.history[c][i].Sent = true
						break
					}
				}
			}
			saveHistory(m.history)
		}
		return m, nil

	case typingMsg:
		if msg.action == "STARTED" {
			m.typing = msg.from
		} else {
			m.typing = ""
		}
		return m, listenEvents(m.port, m.account)

	case messageReceivedMsg:
		debugLog("messageReceivedMsg from=%s text=%s", msg.msg.From, msg.msg.Text)
		// dedup: skip if already present
		for _, existing := range m.messages {
			if existing.Contact == msg.msg.Contact && existing.Time.Equal(msg.msg.Time) {
				return m, nil
			}
		}
		filtered := []Attachment{}
		for _, a := range msg.msg.Attachments {
			if strings.TrimSpace(a.Filename) == "" {
				continue
			}
			filtered = append(filtered, a)
		}
		msg.msg.Attachments = filtered
		m.typing = ""
		m.messages = append(m.messages, msg.msg)
		if m.history == nil {
			m.history = make(map[string][]Message)
		}
		m.history[msg.msg.Contact] = append(m.history[msg.msg.Contact], msg.msg)
		saveHistory(m.history)
		cmds := []tea.Cmd{listenEvents(m.port, m.account), sendReadReceiptCmd(m.port, m.account, msg.msg.Contact, msg.msg.Time.UnixMilli())}
		for _, att := range msg.msg.Attachments {
			if !isAllowedExtension(att.Filename) {
				m.status = "Restricted file detected and removed: " + att.Filename
				notice := Message{From: "System", Text: "Restricted file removed: " + att.Filename, Time: time.Now(), IsMe: false, Contact: msg.msg.Contact}
				m.messages = append(m.messages, notice)
				m.history[msg.msg.Contact] = append(m.history[msg.msg.Contact], notice)
				saveHistory(m.history)
				continue
			}
			cmds = append(cmds, fetchAttachmentCmd(m.port, m.account, msg.msg.Contact, msg.msg.Time, att))
		}
		return m, tea.Batch(cmds...)

	case attachmentReadyMsg:
		if msg.err != nil {
			m.status = "Attachment error: " + msg.err.Error()
			return m, nil
		}
		if !isAllowedExtension(msg.att.Filename) {
			_ = os.Remove(msg.path)
			m.status = "Restricted file detected and removed: " + msg.att.Filename
			return m, nil
		}
		for i := range m.messages {
			if m.messages[i].Contact == msg.contact && m.messages[i].Time.Equal(msg.time) {
				m.messages[i].LocalPaths = append(m.messages[i].LocalPaths, msg.path)
				break
			}
		}
		if msgs, ok := m.history[msg.contact]; ok {
			for i := range msgs {
				if msgs[i].Time.Equal(msg.time) {
					msgs[i].LocalPaths = append(msgs[i].LocalPaths, msg.path)
					break
				}
			}
			m.history[msg.contact] = msgs
			saveHistory(m.history)
		}
		if msg.path != "" {
			m.lastAttachment = msg.path
			m.pendingAttachment = msg.path
			m.pendingAtt = msg.att
		}
		return m, nil

	case saveResultMsg:
		if msg.err != nil {
			m.status = "Save error: " + msg.err.Error()
		} else {
			m.status = "Saved to " + msg.path
		}
		return m, nil

	case qrCodeMsg:
		m.qrCode = msg.qr
		m.linkURL = msg.url
		m.state = stateWaitingLink
		return m, waitForLink(m.beforeAccounts)

	case signalCLIInstalledMsg:
		if msg.err != nil {
			m.setupErr = "Install failed: " + msg.err.Error()
			m.state = stateMissingDep
			return m, nil
		}
		if _, err := exec.LookPath("signal-cli"); err != nil {
			m.setupErr = "signal-cli still not found after install"
			m.state = stateMissingDep
			return m, nil
		}
		m.setupErr = ""
		if m.config != nil && m.config.Account != "" {
			m.state = stateStartingDaemon
			m.setupMsg = "Starting daemon..."
			return m, startDaemon(m.account, m.port)
		}
		m.state = stateSetup
		m.port = findAvailablePort(8081)
		return m, nil

	case linkCompleteMsg:
		if msg.err != nil {
			m.setupErr = msg.err.Error()
			m.state = stateSetup
			return m, nil
		}
		m.account = msg.account
		m.state = stateStartingDaemon
		m.setupMsg = "Starting daemon..."
		return m, startDaemon(m.account, m.port)

	case tea.KeyPressMsg:
		if m.showHelp {
			if msg.String() == "esc" || msg.String() == "ctrl+h" {
				m.showHelp = false
			}
			return m, nil
		}
		if m.fpActive {
			var cmd tea.Cmd
			m.fp, cmd = m.fp.Update(msg)
			if ok2, path := m.fp.DidSelectFile(msg); ok2 && path != "" {
				m.fpActive = false
				m.status = ""
				if m.selected < len(m.contacts) {
					contact := m.contacts[m.selected]
					recipient := contact.UUID
					if recipient == "" {
						recipient = contact.Number
					}
					msg2 := Message{From: "You", Text: "[attachment: " + filepath.Base(path) + "]", Time: time.Now(), IsMe: true, Contact: contact.Number}
					m.messages = append(m.messages, msg2)
					if m.history == nil {
						m.history = make(map[string][]Message)
					}
					m.history[contact.Number] = append(m.history[contact.Number], msg2)
					saveHistory(m.history)
					return m, tea.Batch(cmd, sendMessageWithAttachment(m.port, recipient, "", path))
				}
				return m, cmd
			}
			if ok2, _ := m.fp.DidSelectDisabledFile(msg); ok2 {
				if isAllowedExtension(m.fp.HighlightedPath()) {
					m.status = ""
				} else {
					m.status = "File type not allowed"
				}
				return m, cmd
			}
			if msg.String() == "esc" {
				m.fpActive = false
				return m, nil
			}
			return m, cmd
		}

		if m.state == stateInstalling {
			if msg.String() == "ctrl+c" {
				return m, tea.Quit
			}
			return m, nil
		}

		if m.state == stateMissingDep {
			if msg.String() == "y" {
				m.state = stateInstalling
				m.setupMsg = "Downloading signal-cli..."
				return m, installSignalCLI()
			}
			if msg.String() == "n" || msg.String() == "q" || msg.String() == "ctrl+c" {
				return m, tea.Quit
			}
			return m, nil
		}

		if m.state == stateSetup {
			if msg.String() == "l" {
				m.state = stateWaitingLink
				m.setupMsg = "Generating link..."
				m.beforeAccounts = readAccountUUIDs()
				return m, runLink()
			}
			if msg.String() == "q" || msg.String() == "ctrl+c" {
				return m, tea.Quit
			}
			return m, nil
		}

		if m.state == stateQRCode || m.state == stateWaitingLink {
			if msg.String() == "q" || msg.String() == "ctrl+c" {
				return m, tea.Quit
			}
			return m, nil
		}

		if m.state == stateStartingDaemon {
			if msg.String() == "q" || msg.String() == "ctrl+c" {
				return m, tea.Quit
			}
			return m, nil
		}

		switch msg.String() {
		case "ctrl+c":
			return m, tea.Quit
		case "ctrl+t":
			if m.fpActive {
				return m, nil
			}
			keys := namedThemeKeys()
			if len(keys) == 0 {
				return m, nil
			}
			idx := 0
			if m.themeName != "" {
				for i, k := range keys {
					if k == m.themeName {
						idx = (i + 1) % len(keys)
						break
					}
				}
			}
			m.themeName = keys[idx]
			m.theme = ThemeForName(m.themeName)
			if m.themeName != "" {
				m.status = "Theme: " + m.themeName
			} else {
				m.status = ""
			}
			return m, nil
		case "ctrl+h":
			m.showHelp = !m.showHelp
			return m, nil
		case "ctrl+a":
			m.fpActive = true
			h := m.height * 3 / 10
			if h < 5 {
				h = 5
			}
			m.fp.SetHeight(h)
			m.fpHeight = h
			return m, m.fp.Init()
		case "ctrl+o":
			if m.lastAttachment != "" {
				cmd := exec.Command("xdg-open", m.lastAttachment)
				return m, tea.ExecProcess(cmd, func(err error) tea.Msg {
					if err != nil {
						return sendResultMsg{err: err}
					}
					return nil
				})
			}
			return m, nil
		case "ctrl+d":
			if m.focus == 0 && m.selected < len(m.contacts) {
				c := m.contacts[m.selected]
				if m.confirmDelete {
					delete(m.history, c.Number)
					saveHistory(m.history)
					m.messages = nil
					m.confirmDelete = false
					m.status = "Deleted conversation with " + c.DisplayName()
				} else {
					m.confirmDelete = true
					m.status = "Press ctrl+d again to delete conversation with " + c.DisplayName()
				}
			}
			return m, nil
		case "tab":
			m.focus = (m.focus + 1) % 2
			if m.focus == 1 {
				m.input.Focus()
			} else {
				m.input.Blur()
			}
			return m, nil
		case "up":
			if m.focus == 0 && m.selected > 0 {
				m.selected--
				if m.selected < len(m.contacts) {
					c := m.contacts[m.selected]
					if msgs, ok := m.history[c.Number]; ok {
						m.messages = msgs
					} else {
						m.messages = nil
					}
				}
			}
			return m, nil
		case "down":
			if m.focus == 0 && m.selected < len(m.contacts)-1 {
				m.selected++
				if m.selected < len(m.contacts) {
					c := m.contacts[m.selected]
					if msgs, ok := m.history[c.Number]; ok {
						m.messages = msgs
					} else {
						m.messages = nil
					}
				}
			}
			return m, nil
		case "enter":
			if m.focus == 1 && m.input.Value() != "" {
				text := m.input.Value()
				m.input.SetValue("")
				if m.selected < len(m.contacts) {
					contact := m.contacts[m.selected]
					recipient := contact.UUID
					if recipient == "" {
						recipient = contact.Number
					}
					msg2 := Message{From: "You", Text: text, Time: time.Now(), IsMe: true, Contact: contact.Number}
					m.messages = append(m.messages, msg2)
					if m.history == nil {
						m.history = make(map[string][]Message)
					}
					m.history[contact.Number] = append(m.history[contact.Number], msg2)
					saveHistory(m.history)
					return m, sendMessageWithAttachment(m.port, recipient, text, "")
				}
			}
			return m, nil
		case "ctrl+r":
			if m.focus == 1 && m.input.Value() != "" {
				break
			}
			if len(m.messages) > 0 && m.selected < len(m.contacts) {
				last := m.messages[len(m.messages)-1]
				contact := m.contacts[m.selected]
				recipient := contact.UUID
				if recipient == "" {
					recipient = contact.Number
				}
				targetAuthor := m.account
				if !last.IsMe {
					targetAuthor = last.Contact
				}
				if last.Reactions == nil {
					last.Reactions = make(map[string]int)
				}
				last.Reactions["👍"]++
				m.messages[len(m.messages)-1] = last
				for c := range m.history {
					if c == contact.Number {
						continue
					}
					for i := range m.history[c] {
						if m.history[c][i].Time.Equal(last.Time) {
							if m.history[c][i].Reactions == nil {
								m.history[c][i].Reactions = make(map[string]int)
							}
							m.history[c][i].Reactions["👍"]++
						}
					}
				}
				saveHistory(m.history)
				return m, sendReactionCmd(m.port, m.account, recipient, "👍", targetAuthor, last.Time.UnixMilli(), false)
			}
			return m, nil
		case "ctrl+shift+r":
			if m.focus == 1 && m.input.Value() != "" {
				break
			}
			if len(m.messages) > 0 && m.selected < len(m.contacts) {
				last := m.messages[len(m.messages)-1]
				contact := m.contacts[m.selected]
				recipient := contact.UUID
				if recipient == "" {
					recipient = contact.Number
				}
				targetAuthor := m.account
				if !last.IsMe {
					targetAuthor = last.Contact
				}
				if last.Reactions != nil && last.Reactions["👍"] > 0 {
					last.Reactions["👍"]--
					if last.Reactions["👍"] <= 0 {
						delete(last.Reactions, "👍")
					}
					m.messages[len(m.messages)-1] = last
					for c := range m.history {
						if c == contact.Number {
							continue
						}
						for i := range m.history[c] {
							if m.history[c][i].Time.Equal(last.Time) {
								if m.history[c][i].Reactions != nil && m.history[c][i].Reactions["👍"] > 0 {
									m.history[c][i].Reactions["👍"]--
									if m.history[c][i].Reactions["👍"] <= 0 {
										delete(m.history[c][i].Reactions, "👍")
									}
								}
							}
						}
					}
					saveHistory(m.history)
					return m, sendReactionCmd(m.port, m.account, recipient, "👍", targetAuthor, last.Time.UnixMilli(), true)
				}
			}
			return m, nil
		case "q":
			if m.focus != 1 || m.input.Value() == "" {
				return m, tea.Quit
			}
		}
	}

	if msg, ok := msg.(tea.KeyPressMsg); ok {
		if msg.String() != "ctrl+d" {
			m.confirmDelete = false
		}
	}
	if m.state == stateChat && m.focus == 1 && m.pendingAttachment == "" {
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		if km, ok := msg.(tea.KeyPressMsg); ok && km.String() != "enter" && m.input.Value() != "" {
			if m.selected < len(m.contacts) {
				contact := m.contacts[m.selected]
				recipient := contact.UUID
				if recipient == "" {
					recipient = contact.Number
				}
				cmd = tea.Batch(cmd, sendTypingIndicatorCmd(m.port, recipient))
			}
		}
		return m, cmd
	}
	return m, nil
}

// namedThemeKeys returns the named themes in a stable, deterministic order.
func namedThemeKeys() []string {
	names := make([]string, 0, len(NamedThemes))
	for k := range NamedThemes {
		names = append(names, k)
	}
	sort.Strings(names)
	return names
}

func (m model) View() tea.View {
	var content string
	if m.width == 0 {
		content = "Loading..."
	} else {
		switch m.state {
		case stateMissingDep:
			content = m.viewMissingDep()
		case stateInstalling:
			content = m.viewInstalling()
		case stateSetup:
			content = m.viewSetup()
		case stateQRCode, stateWaitingLink:
			content = m.viewQRCode()
		case stateStartingDaemon:
			content = m.viewStartingDaemon()
		case stateChat:
			content = m.viewChat()
		default:
			content = "Unknown state"
		}
	}
	v := tea.NewView(content)
	v.AltScreen = true
	if m.width > 0 {
		if lm := leftMargin(m); lm > 0 && content != "Loading..." {
			lines := strings.Split(content, "\n")
			for i := range lines {
				lines[i] = strings.Repeat(" ", lm) + lines[i]
			}
			content = strings.Join(lines, "\n")
			v = tea.NewView(content)
			v.AltScreen = true
		}
		if w, h := lipgloss.Size(content); w > m.width || h > m.height {
			log.Printf("layout overflow: got %dx%d, have %dx%d", w, h, m.width, m.height)
			debugLog("layout overflow: got %dx%d, have %dx%d", w, h, m.width, m.height)
		}
	}
	if m.theme.P.Bg != nil {
		v.BackgroundColor = m.theme.P.Bg
	}
	return v
}

func (m model) viewSetup() string {
	var b strings.Builder
	b.WriteString("\n")
	b.WriteString(lipgloss.NewStyle().Bold(true).Render("  Signal TUI — First Run Setup") + "\n\n")
	b.WriteString("  No account configured.\n\n")
	b.WriteString("  Press [l] to link this device to your Signal account.\n")
	b.WriteString("  (Scan a QR code with your phone's Signal app)\n\n")
	if m.setupErr != "" {
		b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Render("  Error: "+m.setupErr) + "\n\n")
	}
	b.WriteString(lipgloss.NewStyle().Faint(true).Render("  [l] Link device  [q] Quit") + "\n")
	return b.String()
}

func (m model) viewInstalling() string {
	var b strings.Builder
	b.WriteString("\n")
	b.WriteString(lipgloss.NewStyle().Bold(true).Render("  Signal TUI — First Run Setup") + "\n\n")
	b.WriteString("  " + m.setupMsg + "\n\n")
	if m.setupErr != "" {
		b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Render("  Error: "+m.setupErr) + "\n\n")
	}
	b.WriteString(lipgloss.NewStyle().Faint(true).Render("  Please wait... (Ctrl+C to cancel)") + "\n")
	return b.String()
}

func (m model) viewMissingDep() string {
	var b strings.Builder
	b.WriteString("\n")
	b.WriteString(lipgloss.NewStyle().Bold(true).Render("  Signal TUI — First Run Setup") + "\n\n")
	b.WriteString("  signal-cli was not found on your PATH.\n\n")
	if m.setupMsg != "" {
		b.WriteString("  " + m.setupMsg + "\n\n")
	}
	if m.setupErr != "" {
		b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Render("  Error: "+m.setupErr) + "\n\n")
	}
	b.WriteString("  Download and install signal-cli to ~/.local/bin? [y/n]\n")
	b.WriteString(lipgloss.NewStyle().Faint(true).Render("  [y] Yes  [n/q] Quit") + "\n")
	return b.String()
}

func (m model) viewQRCode() string {
	var b strings.Builder
	b.WriteString("\n")
	b.WriteString(lipgloss.NewStyle().Bold(true).Render("  Scan this QR code with Signal on your phone") + "\n")
	b.WriteString(lipgloss.NewStyle().Faint(true).Render("  (Settings → Linked Devices → Link New Device)") + "\n\n")
	b.WriteString(m.qrCode + "\n\n")
	if m.state == stateWaitingLink {
		b.WriteString(lipgloss.NewStyle().Faint(true).Render("  Waiting for scan...") + "\n")
	}
	b.WriteString(lipgloss.NewStyle().Faint(true).Render("  [q] Quit") + "\n")
	return b.String()
}

func (m model) viewStartingDaemon() string {
	var b strings.Builder
	b.WriteString("\n")
	b.WriteString(lipgloss.NewStyle().Bold(true).Render("  Starting Signal daemon...") + "\n\n")
	if m.setupErr != "" {
		b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Render("  Error: "+m.setupErr) + "\n")
	}
	return b.String()
}

func contentWidth(m model) int {
	w := m.width
	if w > 160 {
		w = 160
	}
	if w < 8 {
		w = m.width
	}
	if w < 1 {
		w = 1
	}
	return w
}

func leftMargin(m model) int {
	return (m.width - contentWidth(m)) / 2
}

func (m model) sizes() (sw, cw, bodyH int) {
	w := contentWidth(m)
	if w < 8 {
		if w <= 1 {
			sw = 1
			cw = 1
		} else {
			sw = 1
			cw = w - 1
		}
	} else {
		sw = w / 4
		if sw < 24 {
			sw = 24
		}
		if sw > 36 {
			sw = 36
		}
		if sw > w-4 {
			sw = w - 4
			if sw < 4 {
				sw = 4
			}
		}
		cw = w - sw
		if cw < 4 {
			cw = 4
			sw = w - cw
			if sw < 4 {
				sw = 4
			}
		}
	}
	bodyH = m.height - 1
	if bodyH < 1 {
		bodyH = 1
	}
	if bodyH > m.height && m.height > 0 {
		bodyH = m.height
		if bodyH < 1 {
			bodyH = 1
		}
	}
	return
}

func (m model) viewChat() string {
	if m.pendingAttachment != "" {
		att := m.pendingAtt
		title := "Attachment received: " + att.Filename
		keys := "o: open (supported types)  s: save to ~/Downloads/signal-tui  d/Esc: dismiss"
		if !isSupportedAttachment(att) {
			keys = "s: save to ~/Downloads/signal-tui  d/Esc: dismiss"
		}
		box := lipgloss.NewStyle().Border(m.theme.Border).BorderForeground(m.theme.P.Accent).Padding(1, 2).Render(title + "\n" + keys)
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, box)
	}
	if m.confirmDelete && m.selected < len(m.contacts) {
		c := m.contacts[m.selected]
		title := "Delete Conversation?"
		body := "Delete conversation with " + c.DisplayName() + "?\n\nCtrl+D  Confirm delete\nEsc     Cancel"
		box := lipgloss.NewStyle().Border(m.theme.Border).BorderForeground(m.theme.P.Red).Padding(1, 2).Render(title + "\n\n" + body)
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, box)
	}
	if m.showHelp {
		helpText := "Help\n\nCtrl+A  Attach file\nCtrl+H  Toggle help\nEsc     Close\nCtrl+D  Delete conversation\nCtrl+O  Open last attachment\nCtrl+R  React (+1)\nCtrl+Shift+R  Remove (+1)\nTab     Switch pane\n↑/↓     Navigate contacts\nEnter   Send message\nCtrl+C  Quit\nCtrl+T  Next theme"
		box := lipgloss.NewStyle().Border(m.theme.Border).BorderForeground(m.theme.P.Accent).Padding(1, 2).Render(helpText)
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, box)
	}
	if m.width < 30 || m.height < 8 {
		msg := "Terminal too small (need at least 30x8)"
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, msg)
	}
	sw, cw, bodyH := m.sizes()

	// Build contact list, truncating names to the sidebar inner width.
	contactLines := make([]string, 0, len(m.contacts))
	for i, c := range m.contacts {
		line := c.DisplayName()
		if c.Given == "" && c.Name == "" {
			line = prettyNumber(c.Number)
		}
		line = truncateToWidth(line, sw-4)
		if i == m.selected {
			line = m.theme.ChatItemSel.Render(line)
		} else {
			line = m.theme.ChatItem.Render(line)
		}
		contactLines = append(contactLines, line)
	}

	// Scroll the sidebar window to keep the selected contact visible.
	visibleContacts := bodyH - 3 // title takes one row; borders take two
	if visibleContacts < 1 {
		visibleContacts = 1
	}
	start := 0
	if len(contactLines) > visibleContacts {
		start = m.selected - visibleContacts/2
		if start < 0 {
			start = 0
		}
		if start+visibleContacts > len(contactLines) {
			start = len(contactLines) - visibleContacts
		}
		if start < 0 {
			start = 0
		}
	}

	var sb strings.Builder
	sb.WriteString(m.theme.Title.Render("Contacts") + "\n")
	for i := start; i < start+visibleContacts && i < len(contactLines); i++ {
		sb.WriteString(contactLines[i] + "\n")
	}
	sidebar := lipgloss.NewStyle().Width(sw).Height(bodyH).Border(m.theme.Border).BorderForeground(m.theme.P.Border).Padding(0, 1).Render(sb.String())

	var chatContent strings.Builder
	bubbleWidth := cw - 4
	if bubbleWidth < 1 {
		bubbleWidth = 1
	}
	renderWidth := bubbleWidth
	for _, msg := range m.messages {
		name := "You"
		if !msg.IsMe {
			name = msg.From
			if name == "" {
				name = "Unknown"
			}
		}
		prefix := name + ": "
		msgLines := wrapHanging(prefix, msg.Text, renderWidth)
		style := m.theme.MsgIn
		if msg.IsMe {
			style = m.theme.MsgOut
		}
		for _, l := range msgLines {
			chatContent.WriteString(style.Width(renderWidth).Render(l) + "\n")
		}
		for _, p := range msg.LocalPaths {
			chatContent.WriteString("  [Attachment: " + filepath.Base(p) + "]\n")
		}
		if len(msg.Reactions) > 0 {
			var parts []string
			for emoji, count := range msg.Reactions {
				if emoji == "✅" {
					continue
				}
				if count > 0 {
					parts = append(parts, fmt.Sprintf("%s %d", emoji, count))
				}
			}
			if len(parts) > 0 {
				chatContent.WriteString("  " + lipgloss.NewStyle().Foreground(m.theme.P.Tick).Render(strings.Join(parts, "  ")) + "\n")
			}
		}
		if msg.IsMe {
			if msg.Receipt >= 1 {
				chatContent.WriteString("  " + lipgloss.NewStyle().Foreground(m.theme.P.Tick).Render(m.theme.G.Read) + "\n")
			} else if msg.Sent {
				chatContent.WriteString("  " + lipgloss.NewStyle().Foreground(m.theme.P.Tick).Render(m.theme.G.Sent) + "\n")
			}
		}
		chatContent.WriteString("\n")
	}
	if m.typing != "" {
		tw := cw - 4
		if tw < 1 {
			tw = 1
		}
		chatContent.WriteString(lipgloss.Wrap(m.typing+" is typing...", tw, "") + "\n")
	}

	// messages pane (bodyH-3 rows) + input (3 rows)
	msgsH := bodyH - 3
	if msgsH < 1 {
		msgsH = 1
	}
	var msgsPane string
	if m.fpActive {
		wrapWidth := cw - 4
		if wrapWidth < 1 {
			wrapWidth = 1
		}
		fpLines := strings.Split(m.fp.View(), "\n")
		if len(fpLines) > msgsH-2 {
			fpLines = fpLines[:msgsH-2]
			if len(fpLines) < 1 {
				fpLines = []string{""}
			}
		}
		for i := range fpLines {
			if lipgloss.Width(fpLines[i]) > wrapWidth {
				fpLines[i] = truncateToWidth(fpLines[i], wrapWidth)
			}
		}
		msgsPane = lipgloss.NewStyle().Width(cw).Height(msgsH).Border(m.theme.Border).BorderForeground(m.theme.P.Border).Padding(0, 1).Render(strings.Join(fpLines, "\n"))
	} else {
		// truncate chat content to fit msgsH
		wrapWidth := cw - 4
		if wrapWidth < 1 {
			wrapWidth = 1
		}
		// chatContent is already wrapped per-message (hanging indent), so do not
		// re-wrap here — re-wrapping would drop the indent on continuation lines.
		lines := strings.Split(strings.TrimRight(chatContent.String(), "\n"), "\n")
		inner := msgsH - 2 // account for border top/bottom
		if inner < 1 {
			inner = 1
		}
		if len(lines) > inner {
			lines = lines[len(lines)-inner:]
		}
		for len(lines) < inner {
			lines = append(lines, "")
		}
		padded := strings.Join(lines, "\n")
		msgsPane = lipgloss.NewStyle().Height(msgsH).Border(m.theme.Border).BorderForeground(m.theme.P.Border).Padding(0, 1).Render(padded)
	}

	composer := lipgloss.NewStyle().Width(cw).Height(3).Border(m.theme.Border).BorderForeground(m.theme.P.Border).Padding(0, 1).Render(m.input.View())

	statusText := m.status
	if m.typing != "" {
		statusText = m.typing + " is typing..."
	}
	if statusText == "" {
		statusText = fmt.Sprintf("%d contact", len(m.contacts))
		if len(m.contacts) != 1 {
			statusText += "s"
		}
	}
	attachLabel := m.theme.Hint("^A", "attach")
	helpLabel := m.theme.Hint("^H", "help")
	statusBar := m.theme.Status(contentWidth(m), statusText, attachLabel+"  "+helpLabel)

	// combine sidebar + chat column (no status bar inside)
	chatColumn := lipgloss.JoinVertical(lipgloss.Left, msgsPane, composer)
	topRow := lipgloss.JoinHorizontal(lipgloss.Top, sidebar, chatColumn)
	out := lipgloss.JoinVertical(lipgloss.Left, topRow, statusBar)
	if w, h := lipgloss.Size(out); w > m.width || h > m.height {
		log.Printf("layout overflow: got %dx%d, have %dx%d", w, h, m.width, m.height)
	}
	return out
}

// truncateToWidth shortens s so its visible width is at most width,
// appending an ellipsis when truncation occurs.
func truncateToWidth(s string, width int) string {
	if width <= 0 {
		return ""
	}
	if lipgloss.Width(s) <= width {
		return s
	}
	target := width - 1
	if target < 1 {
		target = 1
	}
	var b strings.Builder
	cur := 0
	for _, r := range s {
		rw := lipgloss.Width(string(r))
		if rw < 1 {
			rw = 1
		}
		if cur+rw > target {
			break
		}
		b.WriteRune(r)
		cur += rw
	}
	return b.String() + "…"
}

// wrapHanging splits text into lines with a hanging indent: the first line
// starts with prefix, continuation lines are indented to align under the
// text that follows prefix.
func wrapHanging(prefix, text string, width int) []string {
	indent := strings.Repeat(" ", lipgloss.Width(prefix))
	cap := width - 2 - lipgloss.Width(prefix)
	if cap < 1 {
		cap = 1
	}
	var result []string
	firstPara := true
	for _, para := range strings.Split(text, "\n") {
		wrapped := lipgloss.Wrap(para, cap, "")
		wlines := strings.Split(wrapped, "\n")
		if len(wlines) == 0 {
			wlines = []string{""}
		}
		for i, l := range wlines {
			if firstPara && i == 0 {
				result = append(result, prefix+l)
			} else if l == "" {
				result = append(result, "")
			} else {
				result = append(result, indent+l)
			}
		}
		firstPara = false
	}
	if len(result) == 0 {
		result = append(result, prefix)
	}
	return result
}

func main() {
	os.MkdirAll(cacheDir(), 0700)
	os.MkdirAll(attachmentCacheDir(), 0700)

	// Resolve signal-cli: prefer a working binary over a broken one on PATH.
	// Prefer the newest working signal-cli. /usr/local/bin has 0.14.9 (supports v11 config),
	// /opt/signal-cli has 0.14.5 (v10 only). ~/.local/bin may be a broken GraalVM image.
	candidates := []string{
		"/usr/local/bin/signal-cli",
		"/opt/signal-cli/bin/signal-cli",
		"/usr/bin/signal-cli",
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			dir := filepath.Dir(c)
			os.Setenv("PATH", dir+":"+os.Getenv("PATH"))
			break
		}
	}
	m := newModel()
	p := tea.NewProgram(m)
	if _, err := p.Run(); err != nil {
		fmt.Println("Error:", err)
		os.Exit(1)
	}
	if daemonCmd != nil {
		if daemonCmd.Process != nil {
			daemonCmd.Process.Signal(syscall.SIGTERM)
			daemonCmd.Wait()
		}
	}
}

func prettyNumber(n string) string {
	if len(n) == 12 && strings.HasPrefix(n, "+1") {
		return fmt.Sprintf("+1 (%s) %s-%s", n[2:5], n[5:8], n[8:])
	}
	return n
}
