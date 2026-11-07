//go:build darwin

package main

import (
	_ "embed"
	"fmt"
	"github.com/justynroberts/marquee/internal/nowplaying"
	"github.com/justynroberts/marquee/internal/update"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"fyne.io/systray"

	"github.com/justynroberts/marquee/internal/api"
	"github.com/justynroberts/marquee/internal/emoji"
	"github.com/justynroberts/marquee/internal/engine"
)

//go:embed icons/tray@2x.png
var trayIcon []byte

const launchLabel = "com.fintonlabs.marquee"

// runTray runs Marquee as a menu-bar app. It must be called on the main goroutine.
func runTray() error {
	logDir := filepath.Join(os.Getenv("HOME"), "Library", "Logs", "Marquee")
	_ = os.MkdirAll(logDir, 0o755)
	logFile, err := os.OpenFile(filepath.Join(logDir, "marquee.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		logFile = os.Stderr
	}
	api.Version = version
	a, err := NewApp(AppOptions{DataDir: engine.DefaultDataDir(), SceneSeconds: 20, Log: newLogger(logFile)})
	if err != nil {
		alert("Marquee could not start", err.Error())
		return err
	}
	listenErr := a.Listen()
	systray.Run(func() { trayReady(a, listenErr) }, func() { a.Close() })
	return nil
}

func trayReady(a *App, listenErr error) {
	systray.SetTemplateIcon(trayIcon, trayIcon)
	systray.SetTooltip("Marquee")

	status := systray.AddMenuItem("Starting…", "")
	status.Disable()
	listenNote := systray.AddMenuItem("", "")
	listenNote.Disable()
	listenNote.Hide()
	if listenErr != nil {
		listenNote.SetTitle("Not listening: port 7781 in use")
		listenNote.Show()
	}
	systray.AddSeparator()
	openRemote := systray.AddMenuItem("Open Remote", "The web remote in your browser")

	play := systray.AddMenuItem("Play", "")
	groups := map[string]*systray.MenuItem{}
	for _, g := range []struct{ key, title string }{{"playlist", "Playlists"}, {"panel", "Panels"}, {"scene", "Scenes"}, {"audio", "Audio"}} {
		groups[g.key] = play.AddSubMenuItem(g.title, "")
	}
	type progItem struct {
		name string
		item *systray.MenuItem
	}
	var progs []progItem
	for _, p := range a.E.Programs() {
		parent, ok := groups[p.Group]
		if !ok {
			continue
		}
		it := parent.AddSubMenuItemCheckbox(p.Name, p.Description, false)
		progs = append(progs, progItem{p.Name, it})
		go func(name string, it *systray.MenuItem) {
			for range it.ClickedCh {
				if err := a.E.Play(name); err != nil {
					alert("Could not play "+name, err.Error())
				}
			}
		}(p.Name, it)
	}

	bright := systray.AddMenuItem("Brightness", "")
	levels := []int{100, 75, 50, 25, 10}
	brightItems := make([]*systray.MenuItem, len(levels))
	for i, l := range levels {
		brightItems[i] = bright.AddSubMenuItemCheckbox(fmt.Sprintf("%d%%", l), "", false)
		go func(l int, it *systray.MenuItem) {
			for range it.ClickedCh {
				a.E.SetBrightness(l)
			}
		}(l, brightItems[i])
	}
	pause := systray.AddMenuItemCheckbox("Pause", "Stop sending frames to the panel", false)
	flip := systray.AddMenuItemCheckbox("Upside Down", "The panel is mounted the other way up", false)
	hudItem := systray.AddMenuItemCheckbox("HUD Mode", "Big clock, high contrast, mirrored to read reflected in your screen", false)
	hudLyr := systray.AddMenuItemCheckbox("HUD Lyrics", "Show the playing song's lyrics on the HUD instead of the clock", false)
	lyrTiming := systray.AddMenuItem("Lyrics Timing", "Show lyric lines earlier or later")
	lyrEarlier := lyrTiming.AddSubMenuItem("Earlier (0.1 s)", "Lines appear sooner")
	lyrLater := lyrTiming.AddSubMenuItem("Later (0.1 s)", "Lines appear later")
	lyrReset := lyrTiming.AddSubMenuItem("Reset", "")
	showLead := func() {
		lyrTiming.SetTitle(fmt.Sprintf("Lyrics Timing: %.2f s early", nowplaying.Lead()))
	}
	showLead()
	go func() {
		for {
			select {
			case <-lyrEarlier.ClickedCh:
				a.E.SetLyricsLead(nowplaying.Lead() + 0.1)
			case <-lyrLater.ClickedCh:
				a.E.SetLyricsLead(nowplaying.Lead() - 0.1)
			case <-lyrReset.ClickedCh:
				a.E.SetLyricsLead(nowplaying.DefaultLead)
			}
			showLead()
		}
	}()
	clearOv := systray.AddMenuItem("Clear Alert", "Cancel a flash or timer")

	systray.AddSeparator()
	s := a.Settings()
	lan := systray.AddMenuItemCheckbox("Allow Other Devices", "Listen on the network (API key required) and advertise via Bonjour", s.LAN)
	reqKey := systray.AddMenuItemCheckbox("Require Key on This Mac", "Apps on this Mac need an API key too", s.RequireKey)
	newKey := systray.AddMenuItem("New API Key…", "Create a key and copy it to the clipboard")
	copyAddr := systray.AddMenuItem("Copy Network Address", "")
	apiRef := systray.AddMenuItem("API Reference", "")

	systray.AddSeparator()
	login := systray.AddMenuItemCheckbox("Open at Login", "", loginEnabled())
	dataDir := systray.AddMenuItem("Show Data Folder", "messages.txt, schedule.txt, media")
	updItem := systray.AddMenuItem("Update Available", "Download the new version")
	updItem.Hide()
	checkUpd := systray.AddMenuItem("Check for Updates…", "")
	var updURL string
	var updMu sync.Mutex
	showUpdate := func(r update.Result) {
		updMu.Lock()
		defer updMu.Unlock()
		if !r.Available || r.Latest == nil {
			updItem.Hide()
			return
		}
		updURL = r.Latest.AssetURL
		if updURL == "" {
			updURL = r.Latest.PageURL
		}
		updItem.SetTitle("Download Marquee " + r.Latest.Version + "…")
		updItem.Show()
	}
	a.updMu.Lock()
	a.onUpdate = showUpdate
	a.updMu.Unlock()
	a.StartUpdateChecks()
	go func() {
		for {
			select {
			case <-updItem.ClickedCh:
				updMu.Lock()
				u := updURL
				updMu.Unlock()
				if u != "" {
					open(u)
				}
			case <-checkUpd.ClickedCh:
				r := a.CheckUpdate(true)
				switch {
				case r.Error != "":
					alert("Could not check for updates", r.Error)
				case r.Available:
					alert("Marquee "+r.Latest.Version+" is available", "Choose Download Marquee "+r.Latest.Version+"… in the menu, open the disk image and drag Marquee to Applications.")
				default:
					alert("Marquee is up to date", "You have version "+version+".")
				}
			}
		}
	}()
	about := systray.AddMenuItem("About Marquee", "")
	quit := systray.AddMenuItem("Quit Marquee", "")

	refresh := func() {
		st := a.E.Status()
		panel := "Panel not found"
		switch st.Panel {
		case "connected":
			panel = "Panel connected"
		case "disabled":
			panel = "Panel disabled"
		}
		title := fmt.Sprintf("%s · %s", panel, st.Program)
		if st.Overlay != "" {
			title += " · " + st.Overlay
		}
		if st.Paused {
			title += " · paused"
		}
		status.SetTitle(title)
		for _, p := range progs {
			if p.name == st.Program {
				p.item.Check()
			} else {
				p.item.Uncheck()
			}
		}
		for i, l := range levels {
			if st.Brightness == l {
				brightItems[i].Check()
			} else {
				brightItems[i].Uncheck()
			}
		}
		setCheck(pause, st.Paused)
		setCheck(flip, st.Flip)
		setCheck(hudItem, st.Program == "hud")
		setCheck(hudLyr, st.HUDLyrics)
		showLead()
		s := a.Settings()
		setCheck(lan, s.LAN)
		setCheck(reqKey, s.RequireKey)
		if s.LAN {
			copyAddr.Show()
		} else {
			copyAddr.Hide()
		}
		if err := a.ListenError(); err != nil {
			listenNote.SetTitle("Not listening: " + shorten(err.Error(), 60))
			listenNote.Show()
		} else {
			listenNote.Hide()
		}
	}
	refresh()
	events, _ := a.E.Subscribe()
	go func() {
		tick := time.NewTicker(3 * time.Second)
		for {
			select {
			case <-events:
			case <-tick.C:
			}
			refresh()
		}
	}()

	for {
		select {
		case <-openRemote.ClickedCh:
			open(a.RemoteURL())
		case <-pause.ClickedCh:
			a.E.SetPaused(!pause.Checked())
		case <-flip.ClickedCh:
			a.E.SetFlip(!flip.Checked())
		case <-hudItem.ClickedCh:
			a.E.SetHUD(!a.E.HUDOn())
		case <-hudLyr.ClickedCh:
			on := !hudLyr.Checked()
			a.E.SetHUDLyrics(on)
			if on {
				a.E.SetHUD(true)
			}
		case <-clearOv.ClickedCh:
			a.E.ClearOverlay()
		case <-lan.ClickedCh:
			on := !lan.Checked()
			if err := a.Update(func(s *Settings) { s.LAN = on }); err != nil {
				alert("Could not change network access", err.Error())
			} else if on && len(a.Keys.List()) == 0 {
				alert("Other devices can now reach Marquee",
					"They need an API key. Choose New API Key… to create one, then send it as\n\nAuthorization: Bearer mq_…")
			}
			refresh()
		case <-reqKey.ClickedCh:
			on := !reqKey.Checked()
			if err := a.Update(func(s *Settings) { s.RequireKey = on }); err != nil {
				alert("Could not change the setting", err.Error())
			}
			refresh()
		case <-newKey.ClickedCh:
			name, ok := prompt("Name this API key (the app or device that will use it):", "")
			if !ok {
				continue
			}
			if strings.TrimSpace(name) == "" {
				name = "unnamed"
			}
			_, secret, err := a.Keys.Create(name)
			if err != nil {
				alert("Could not create a key", err.Error())
				continue
			}
			copyText(secret)
			alert("API key copied to the clipboard",
				"Store it now; Marquee keeps only a hash and cannot show it again.\n\n"+secret+
					"\n\nSend it as: Authorization: Bearer <key>\nRevoke keys with: marquee key list / key revoke ID")
		case <-copyAddr.ClickedCh:
			if urls := LANURLs(); len(urls) > 0 {
				copyText(urls[0])
			}
		case <-apiRef.ClickedCh:
			open(a.RemoteURL() + "/v1/openapi.json")
		case <-login.ClickedCh:
			if err := setLogin(!login.Checked()); err != nil {
				alert("Could not change Open at Login", err.Error())
			}
			setCheck(login, loginEnabled())
		case <-dataDir.ClickedCh:
			open(a.Paths.Dir)
		case <-about.ClickedCh:
			alert("Marquee "+version,
				"Drives Thermalright USB LED panels: scenes, a message board, audio visualisers, a dashboard, alerts and pictures, with an HTTP API for other apps.\n\n"+
					"Made by FintonLabs. MIT licence.\n"+emoji.Attribution+".\nNot affiliated with Thermalright.")
		case <-quit.ClickedCh:
			systray.Quit()
			return
		}
	}
}

func setCheck(m *systray.MenuItem, on bool) {
	if on {
		m.Check()
	} else {
		m.Uncheck()
	}
}

func shorten(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}

func open(target string) { _ = exec.Command("open", target).Start() }

func copyText(s string) {
	cmd := exec.Command("pbcopy")
	cmd.Stdin = strings.NewReader(s)
	_ = cmd.Run()
}

// osaQuote makes s safe inside an AppleScript string literal.
func osaQuote(s string) string {
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(s) + `"`
}

func alert(title, msg string) {
	script := fmt.Sprintf(`display dialog %s with title %s buttons {"OK"} default button "OK"`, osaQuote(msg), osaQuote(title))
	go func() { _ = exec.Command("osascript", "-e", script).Run() }()
}

func prompt(question, def string) (string, bool) {
	script := fmt.Sprintf(`text returned of (display dialog %s default answer %s with title "Marquee")`, osaQuote(question), osaQuote(def))
	out, err := exec.Command("osascript", "-e", script).Output()
	if err != nil {
		return "", false // cancelled
	}
	return strings.TrimSpace(string(out)), true
}

// ---------------------------------------------------------------- open at login (LaunchAgent)

func launchAgentPath() string {
	return filepath.Join(os.Getenv("HOME"), "Library", "LaunchAgents", launchLabel+".plist")
}

func loginEnabled() bool {
	_, err := os.Stat(launchAgentPath())
	return err == nil
}

// appBundle returns the enclosing .app, if running from one.
func appBundle() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	exe, _ = filepath.EvalSymlinks(exe)
	if i := strings.Index(exe, ".app/Contents/MacOS/"); i >= 0 {
		return exe[:i+4]
	}
	return ""
}

func setLogin(on bool) error {
	path := launchAgentPath()
	if !on {
		return os.Remove(path)
	}
	bundle := appBundle()
	if bundle == "" {
		return fmt.Errorf("Open at Login works when Marquee runs from Marquee.app")
	}
	plist := `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key><string>` + launchLabel + `</string>
  <key>ProgramArguments</key>
  <array><string>/usr/bin/open</string><string>-a</string><string>` + xmlEscape(bundle) + `</string></array>
  <key>RunAtLoad</key><true/>
</dict>
</plist>
`
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(plist), 0o644)
}

func xmlEscape(s string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;").Replace(s)
}

func inAppBundle() bool { return appBundle() != "" }
