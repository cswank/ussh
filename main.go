package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"

	ui "github.com/jroimartin/gocui"
	"gopkg.in/alecthomas/kingpin.v2"
)

var (
	query    = kingpin.Arg("query", "search string").String()
	addr     = os.Getenv("UPTIME_ADDR")
	username = os.Getenv("UPTIME_USER")
	secret   = os.Getenv("UPTIME_KEY")
	current  string
	cursor   int
	disp     []host
	chars    = "abcdefghijklmnopqrstuvwzyzABCDEFGHIJKLMNOPQRSTUVWZYZ1234567890.-,"
)

type host struct {
	name     string
	selected bool
	hide     bool
}

type result struct {
	Host string `json:"fqdn"`
}

type uptime struct {
	Message string   `json:"message"`
	Results []result `json:"results"`
}

func init() {
	f, _ := os.Create("/tmp/ussh")
	log.SetOutput(f)
}

func main() {
	kingpin.Parse()
	hosts := getHosts()
	targets := getTargets(hosts)
	login(targets)
}

func inBoth(h string, preds []string) bool {
	for _, p := range preds {
		if strings.Index(h, p) == -1 {
			return false
		}
	}
	return true
}

func search(hosts []host, pred string) {
	pred = strings.TrimSpace(pred)
	preds := strings.Split(pred, ",")

	for i, h := range hosts {
		if inBoth(h.name, preds) {
			hosts[i].hide = false
		} else {
			hosts[i].hide = true
		}
	}
}

func acceptable(s string) bool {
	return strings.Index(chars, s) > -1
}

func getTargets(hosts []string) []string {
	disp = make([]host, len(hosts))
	for i, h := range hosts {
		disp[i] = host{name: h}
	}

	g := ui.NewGui()
	if err := g.Init(); err != nil {
		log.Panicln(err)
	}

	g.FgColor = ui.ColorGreen

	current = "hosts-cursor"

	g.Editor = ui.EditorFunc(edit)

	defer g.Close()

	if err := keybindings(g); err != nil {
		log.Panicln(err)
	}

	g.SetLayout(layout)

	g.SelFgColor = ui.ColorGreen
	g.Cursor = true

	g.MainLoop()
	t, err := g.View("targets")
	if err != nil {
		return []string{}
	}

	return doGetTargets(strings.Split(t.Buffer(), "\n"))
}

func layout(g *ui.Gui) error {
	x, _ := g.Size()
	size := len(disp)

	if v, err := g.SetView("hosts-label", -1, -1, x, 1); err != nil {
		if err != ui.ErrUnknownView {
			return err
		}
		v.Frame = false
		v.FgColor = ui.ColorGreen
		fmt.Fprint(v, "hosts:")
	}

	if v, err := g.SetView("hosts-cursor", 4, 0, 6, size+1); err != nil {
		if err != ui.ErrUnknownView {
			return err
		}
		v.Highlight = true
		v.Frame = false
		for _, h := range disp {
			if h.hide {
				continue
			}
			if h.selected {
				fmt.Fprintln(v, "x")
			} else {
				fmt.Fprintln(v, " ")
			}
		}
	}

	if v, err := g.SetView("hosts", 6, 0, x, size+1); err != nil {
		if err != ui.ErrUnknownView {
			return err
		}
		v.Frame = false
		for _, h := range disp {
			if h.hide {
				continue
			}
			var tpl string

			if h.selected {
				tpl = "\033[32m%s\033[37m\n"
			} else {
				tpl = "\033[37m%s\033[37m\n"
			}
			fmt.Fprintf(v, tpl, h.name)
		}
	}

	if v, err := g.SetView("filter-label", -1, size, x, size+2); err != nil {
		if err != ui.ErrUnknownView {
			return err
		}
		v.FgColor = ui.ColorGreen
		v.Highlight = false
		v.Frame = false
		v.Editable = false
		fmt.Fprintln(v, "filter: ")
	}

	if v, err := g.SetView("filter", 4, size+1, x, size+3); err != nil {
		if err != ui.ErrUnknownView {
			return err
		}
		v.FgColor = ui.ColorGreen
		v.Highlight = false
		v.Frame = false
		v.Editable = true
	}

	g.SetCurrentView(current)
	return nil
}

func edit(v *ui.View, key ui.Key, ch rune, mod ui.Modifier) {
	if key == ui.KeyEnter {
		current = "hosts-cursor"
		return
	}
	s := strings.TrimSpace(v.Buffer())
	if key == 127 && len(s) > 0 {
		v.Clear()
		s = s[:len(s)-1]
		v.Write([]byte(s))
		search(disp, s)
		v.SetCursor(len(s), 0)
	} else if key == 127 && len(s) == 0 {
		for i, _ := range disp {
			disp[i].hide = false
		}
	} else if acceptable(string(ch)) {
		fmt.Fprint(v, string(ch))
		s = v.Buffer()
		search(disp, s)
		v.SetCursor(len(s)-1, 0)
	}
	// 	hv, _ := g.View("hosts")
	// hv.Clear()
	// for _, h := range disp {
	// 	if !h.hide {
	// 		fmt.Fprintln(hv, h.name)
	// 	}
	// }
}

func doGetTargets(hosts []string) []string {
	var out []string
	for _, targ := range hosts {
		if strings.Index(targ, "hosts") == -1 {
			h := strings.TrimSpace(targ)
			if len(h) > 0 {
				out = append(out, h)
			}
		}
	}
	return out
}

func quit(g *ui.Gui, v *ui.View) error {
	t, err := g.View("targets")
	if err != nil {
		return err
	}
	t.Clear()
	return ui.ErrQuit
}

func filter(g *ui.Gui, v *ui.View) error {
	v, _ = g.View("filter")
	buf := v.Buffer()
	v.SetCursor(0, len(buf))
	current = "filter"
	return nil
}

func doFilter(g *ui.Gui, v *ui.View) error {
	current = "hosts-cursor"
	return nil
}

func sshAll(g *ui.Gui, v *ui.View) error {
	return ui.ErrQuit
}

func next(g *ui.Gui, v *ui.View) error {
	cursor++
	cx, cy := v.Cursor()
	return v.SetCursor(cx, cy+1)
}

func prev(g *ui.Gui, v *ui.View) error {
	cursor--
	cx, cy := v.Cursor()
	return v.SetCursor(cx, cy-1)
}

func ssh(g *ui.Gui, v *ui.View) error {
	_, cy := v.Cursor()
	l, err := v.Line(cy)
	if err != nil {
		return err
	}
	t, err := g.View("targets")
	if err != nil {
		return err
	}
	t.Clear()
	_, err = fmt.Fprintln(t, l)
	if err != nil {
		return err
	}
	return ui.ErrQuit
}

func sel(g *ui.Gui, v *ui.View) error {
	_, cy := v.Cursor()
	v.Clear()
	log.Println("sel", v.Name, cy)
	disp[cy].selected = true
	return nil
}

type key struct {
	name string
	key  interface{}
	mod  ui.Modifier
	f    ui.KeybindingHandler
}

var keys = []key{
	{"", ui.KeyCtrlC, ui.ModNone, quit},
	{"", ui.KeyCtrlD, ui.ModNone, quit},
	{"", ui.KeyCtrlA, ui.ModNone, sshAll},
	{"", ui.KeyCtrlF, ui.ModNone, filter},
	{"filter", ui.KeyEnter, ui.ModNone, doFilter},
	{"hosts-cursor", ui.KeyCtrlN, ui.ModNone, next},
	{"hosts-cursor", ui.KeyCtrlP, ui.ModNone, prev},
	{"hosts-cursor", ui.KeySpace, ui.ModNone, sel},
	{"hosts-cursor", ui.KeyEnter, ui.ModNone, ssh},
}

func keybindings(g *ui.Gui) error {
	for _, k := range keys {
		if err := g.SetKeybinding(k.name, k.key, k.mod, k.f); err != nil {
			return err
		}
	}
	return nil
}

func login(targets []string) {
	if len(targets) == 1 && targets[0] != "" {
		cmd := exec.Command("ssh", fmt.Sprintf("%s@%s", username, targets[0]))
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		err := cmd.Run()
		if err != nil {
			log.Fatal("couldn't ssh", err)
		}
	} else if len(targets) > 1 {
		for i, x := range targets {
			targets[i] = fmt.Sprintf("%s@%s", username, x)
		}
		cmd := exec.Command("csshx", targets...)
		err := cmd.Run()
		if err != nil {
			log.Fatal(err)
		}
	}
}

func getHosts() []string {
	f := []string{"p1", "s1"}
	c := []string{"a", "b", "c", "d", "e"}
	out := make([]string, len(c))
	for i, x := range c {
		out[i] = fmt.Sprintf("%s%s", strings.Repeat(x, 10), f[i%2])
	}
	return out
}

// func getHosts() []string {
// 	q := strings.Replace(*query, "%", "%25", -1)
// 	resp, err := http.Get(fmt.Sprintf("%s/servers/search/%s/%s", addr, q, secret))
// 	if err != nil {
// 		log.Fatal(err)
// 	}
// 	defer resp.Body.Close()

// 	var u uptime
// 	dec := json.NewDecoder(resp.Body)
// 	if err := dec.Decode(&u); err != nil {
// 		log.Fatal(err)
// 	}
// 	hosts := make([]string, len(u.Results))

// 	for i, x := range u.Results {
// 		hosts[i] = x.Host
// 	}

// 	sort.Strings(hosts)
// 	return hosts
// }
