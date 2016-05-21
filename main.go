package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"sort"
	"strings"

	"github.com/jroimartin/gocui"
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

func getTargets(hosts []string) []string {
	g := gocui.NewGui()
	if err := g.Init(); err != nil {
		log.Panicln(err)
	}
	defer g.Close()

	if err := keybindings(g); err != nil {
		log.Panicln(err)
	}

	current = "hosts"

	g.SetLayout(func(g *gocui.Gui) error {
		x, y := g.Size()
		size := len(hosts)

		if v, err := g.SetView("hosts-label", -1, -1, x, 1); err != nil {
			if err != gocui.ErrUnknownView {
				return err
			}
			v.Frame = false
			v.FgColor = gocui.ColorGreen
			fmt.Fprint(v, "hosts:")
		}

		if v, err := g.SetView("hosts", 4, 0, x, size+1); err != nil {
			if err != gocui.ErrUnknownView {
				return err
			}
			v.FgColor = gocui.ColorGreen
			v.Highlight = true
			v.Frame = false
			v.SelFgColor = gocui.ColorWhite
			for _, h := range hosts {
				fmt.Fprintln(v, h)
			}
			v.SetCursor(0, 0)
		}

		if v, err := g.SetView("filter-label", -1, size, x, size+2); err != nil {
			if err != gocui.ErrUnknownView {
				return err
			}
			v.FgColor = gocui.ColorGreen
			v.Highlight = false
			v.Frame = false
			v.Editable = false
			v.SelFgColor = gocui.ColorWhite
			fmt.Fprintln(v, "filter: ")
		}

		if v, err := g.SetView("filter", 4, size+1, x, size+3); err != nil {
			if err != gocui.ErrUnknownView {
				return err
			}
			v.FgColor = gocui.ColorGreen
			v.Highlight = false
			v.Frame = false
			v.Editable = true
		}

		if v, err := g.SetView("ssh-label", -1, size+2, x, size+4); err != nil {
			if err != gocui.ErrUnknownView {
				return err
			}
			v.FgColor = gocui.ColorGreen
			v.Highlight = false
			v.Frame = false
			v.Editable = false
			v.SelFgColor = gocui.ColorWhite
			fmt.Fprint(v, "ssh to:")
		}

		if v, err := g.SetView("targets", 4, size+3, x, y); err != nil {
			if err != gocui.ErrUnknownView {
				return err
			}
			v.FgColor = gocui.ColorGreen
			v.Highlight = true
			v.Frame = false
			v.SelFgColor = gocui.ColorGreen
		}
		g.SetCurrentView(current)
		return nil

	})

	g.SelFgColor = gocui.ColorGreen
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

func quit(g *gocui.Gui, v *gocui.View) error {
	t, err := g.View("targets")
	if err != nil {
		return err
	}
	t.Clear()
	return gocui.ErrQuit
}

func filter(g *gocui.Gui, v *gocui.View) error {
	v, _ = g.View("filter")
	buf := v.Buffer()
	v.SetCursor(0, len(buf))
	current = "filter"
	return nil
}

func doFilter(g *gocui.Gui, v *gocui.View) error {
	v, _ = g.View("filter")
	buf := v.Buffer()
	log.Println(buf)
	current = "hosts"
	return nil
}

func sshAll(g *gocui.Gui, v *gocui.View) error {
	return gocui.ErrQuit
}

func next(g *gocui.Gui, v *gocui.View) error {
	cx, cy := v.Cursor()
	return v.SetCursor(cx, cy+1)
}

func prev(g *gocui.Gui, v *gocui.View) error {
	cx, cy := v.Cursor()
	return v.SetCursor(cx, cy-1)
}

func ssh(g *gocui.Gui, v *gocui.View) error {
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
	return gocui.ErrQuit
}

func sel(g *gocui.Gui, v *gocui.View) error {
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
	mod  gocui.Modifier
	f    gocui.KeybindingHandler
}

var keys = []key{
	{"", gocui.KeyCtrlC, gocui.ModNone, quit},
	{"", gocui.KeyCtrlD, gocui.ModNone, quit},
	{"", gocui.KeyCtrlA, gocui.ModNone, sshAll},
	{"", gocui.KeyCtrlF, gocui.ModNone, filter},
	{"filter", gocui.KeyEnter, gocui.ModNone, doFilter},
	{"hosts", gocui.KeyCtrlN, gocui.ModNone, next},
	{"hosts", gocui.KeyCtrlP, gocui.ModNone, prev},
	{"hosts", gocui.KeySpace, gocui.ModNone, sel},
	{"hosts", gocui.KeyEnter, gocui.ModNone, ssh},
}

func keybindings(g *gocui.Gui) error {
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
	q := strings.Replace(*query, "%", "%25", -1)
	resp, err := http.Get(fmt.Sprintf("%s/servers/search/%s/%s", addr, q, secret))
	if err != nil {
		log.Fatal(err)
	}
	defer resp.Body.Close()

	var u uptime
	dec := json.NewDecoder(resp.Body)
	if err := dec.Decode(&u); err != nil {
		log.Fatal(err)
	}
	hosts := make([]string, len(u.Results))

	for i, x := range u.Results {
		hosts[i] = x.Host
	}

	sort.Strings(hosts)
	return hosts
}
