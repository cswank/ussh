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
)

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

func search(hosts []string, pred string) []string {
	pred = strings.TrimSpace(pred)
	preds := strings.Split(pred, ",")

	var out []string
	for _, h := range hosts {
		if inBoth(h, preds) {
			out = append(out, h)
		}
	}
	return out
}

func getTargets(hosts []string) []string {
	disp := make([]string, len(hosts))
	copy(disp, hosts)

	g := ui.NewGui()

	if err := g.Init(); err != nil {
		log.Panicln(err)
	}

	g.Editor = ui.EditorFunc(func(v *ui.View, key ui.Key, ch rune, mod ui.Modifier) {
		if key == ui.KeyEnter {
			current = "hosts"
			return
		}

		s := strings.TrimSpace(v.Buffer())
		if key == 127 && len(s) > 0 {
			v.Clear()
			s = s[:len(s)-1]
			v.Write([]byte(s))
			disp = search(hosts, s)
			v.SetCursor(len(s), 0)
		} else {
			fmt.Fprint(v, string(ch))
			s = v.Buffer()
			disp = search(hosts, s)
			log.Println(s, disp)
			v.SetCursor(len(s)-1, 0)
		}

		hv, _ := g.View("hosts")
		hv.Clear()
		if len(s) == 0 {
			disp = make([]string, len(hosts))
			copy(disp, hosts)
		}
		for _, h := range disp {
			fmt.Fprintln(hv, h)
		}
	})

	defer g.Close()

	if err := keybindings(g); err != nil {
		log.Panicln(err)
	}

	current = "hosts"

	g.SetLayout(func(g *ui.Gui) error {
		x, y := g.Size()
		size := len(hosts)

		if v, err := g.SetView("hosts-label", -1, -1, x, 1); err != nil {
			if err != ui.ErrUnknownView {
				return err
			}
			v.Frame = false
			v.FgColor = ui.ColorGreen
			fmt.Fprint(v, "hosts:")
		}

		if v, err := g.SetView("hosts", 4, 0, x, size+1); err != nil {
			if err != ui.ErrUnknownView {
				return err
			}
			v.FgColor = ui.ColorGreen
			v.Highlight = true
			v.Frame = false
			v.SelFgColor = ui.ColorWhite
			for _, h := range disp {
				fmt.Fprintln(v, h)
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
			v.SelFgColor = ui.ColorWhite
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

		if v, err := g.SetView("ssh-label", -1, size+2, x, size+4); err != nil {
			if err != ui.ErrUnknownView {
				return err
			}
			v.FgColor = ui.ColorGreen
			v.Highlight = false
			v.Frame = false
			v.Editable = false
			v.SelFgColor = ui.ColorWhite
			fmt.Fprint(v, "ssh to:")
		}

		if v, err := g.SetView("targets", 4, size+3, x, y); err != nil {
			if err != ui.ErrUnknownView {
				return err
			}
			v.FgColor = ui.ColorGreen
			v.Highlight = true
			v.Frame = false
			v.SelFgColor = ui.ColorGreen
		}
		g.SetCurrentView(current)
		return nil

	})

	g.SelFgColor = ui.ColorGreen
	g.Cursor = true

	g.MainLoop()
	t, err := g.View("targets")
	if err != nil {
		return []string{}
	}

	return doGetTargets(strings.Split(t.Buffer(), "\n"))
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
	v, _ = g.View("filter")
	current = "hosts"
	return nil
}

func sshAll(g *ui.Gui, v *ui.View) error {
	return ui.ErrQuit
}

func next(g *ui.Gui, v *ui.View) error {
	cx, cy := v.Cursor()
	return v.SetCursor(cx, cy+1)
}

func prev(g *ui.Gui, v *ui.View) error {
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
	l, err := v.Line(cy)
	if err != nil {
		return err
	}
	t, err := g.View("targets")
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(t, l)
	return err
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
	{"hosts", ui.KeyCtrlN, ui.ModNone, next},
	{"hosts", ui.KeyCtrlP, ui.ModNone, prev},
	{"hosts", ui.KeySpace, ui.ModNone, sel},
	{"hosts", ui.KeyEnter, ui.ModNone, ssh},
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
