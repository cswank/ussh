package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"

	ui "github.com/jroimartin/gocui"
	"github.com/marpaia/chef-golang"
	"gopkg.in/alecthomas/kingpin.v2"
)

var (
	g         *ui.Gui
	query     = kingpin.Arg("query", "search string").String()
	role      = kingpin.Flag("role", "chef role").Short('r').String()
	addr      = os.Getenv("UPTIME_ADDR")
	username  = os.Getenv("UPTIME_USER")
	secret    = os.Getenv("UPTIME_KEY")
	current   string
	hosts     []host
	names     []string
	chars     = "abcdefghijklmnopqrstuvwzyzABCDEFGHIJKLMNOPQRSTUVWZYZ1234567890.-,"
	templates = map[string]string{
		"green":  "\033[32m%s\033[37m\n",
		"white":  "\033[37m%s\033[37m\n",
		"yellow": "\033[33m%s\033[37m\n",
	}
)

type host struct {
	name     string
	selected bool
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
	names = getHosts()
	targets := getTargets()
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

func search(pred string) {
	_, my := g.Size()
	my -= 5
	pred = strings.TrimSpace(pred)
	preds := strings.Split(pred, ",")

	out := []host{}
	for i, n := range names {
		if i < my && inBoth(n, preds) {
			out = append(out, host{name: n})
		}
	}
	hosts = out
}

func acceptable(s string) bool {
	return strings.Index(chars, s) > -1
}

func makeHosts() []host {
	_, my := g.Size()
	my -= 5
	var out []host
	for i, h := range names {
		if i < my {
			out = append(out, host{name: h})
		}
	}
	return out
}

func getTargets() []string {
	g = ui.NewGui()
	if err := g.Init(); err != nil {
		log.Panicln(err)
	}

	hosts = makeHosts()

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

	if err := g.MainLoop(); err != nil {
		if err != ui.ErrQuit {
			log.Fatal(err)
		}
	}
	var out []string
	for _, h := range hosts {
		if h.selected {
			out = append(out, h.name)
		}
	}
	return out
}

func layout(g *ui.Gui) error {
	log.Println("layout")
	x, _ := g.Size()
	size := len(hosts)

	if v, err := g.SetView("hosts-label", -1, -1, x, 1); err != nil {
		if err != ui.ErrUnknownView {
			return err
		}
		v.Frame = false
		//v.FgColor = ui.ColorGreen
		fmt.Fprint(v, "\033[37mhosts\033[37m\n")
	}

	if v, err := g.SetView("hosts-cursor", 4, 0, 6, size+1); err != nil {
		if err != ui.ErrUnknownView {
			return err
		}
		v.Highlight = true
		v.Frame = false
		for _, h := range hosts {
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
		printHosts()
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

	return g.SetCurrentView(current)
}

func printHosts() {
	hv, _ := g.View("hosts")
	cv, _ := g.View("hosts-cursor")
	hv.Clear()
	_, cur := cv.Cursor()
	for i, h := range hosts {
		color := "green"
		if h.selected && i == cur {
			color = "yellow"
		} else if h.selected || i == cur {
			color = "white"
		}
		fmt.Fprintf(hv, templates[color], h.name)
	}
}

func edit(v *ui.View, key ui.Key, ch rune, mod ui.Modifier) {
	log.Println("edit", key, ch, key == ui.KeyEnter)
	if key == ui.KeyEnter {
		cv, _ := g.View("hosts-cursor")
		cv.SetCursor(0, 0)
		current = "hosts-cursor"
		return
	}
	s := strings.TrimSpace(v.Buffer())
	if key == 127 && len(s) > 0 {
		v.Clear()
		s = s[:len(s)-1]
		v.Write([]byte(s))
		search(s)
		v.SetCursor(len(s), 0)
	} else if key == 127 && len(s) == 0 {
		hosts = makeHosts()
	} else if acceptable(string(ch)) {
		fmt.Fprint(v, string(ch))
		s = v.Buffer()
		search(s)
		v.SetCursor(len(s)-1, 0)
	}
	printHosts()
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
	return ui.ErrQuit
}

func filter(g *ui.Gui, v *ui.View) error {
	var err error
	if v, err = g.View("filter"); err != nil {
		return err
	}
	buf := strings.TrimSpace(v.Buffer())
	v.Clear()

	v.Write([]byte(buf))
	current = "filter"
	l := len(buf)
	err = v.SetCursor(l, 0)
	maxX, maxY := v.Size()
	log.Println("filter", l, buf, err, maxX, maxY)
	return err
}

func doFilter(g *ui.Gui, v *ui.View) error {
	current = "hosts-cursor"
	return nil
}

func sshAll(g *ui.Gui, v *ui.View) error {
	for i := range hosts {
		hosts[i].selected = true
	}
	return ui.ErrQuit
}

func next(g *ui.Gui, v *ui.View) error {
	cx, cy := v.Cursor()
	err := v.SetCursor(cx, cy+1)
	printHosts()
	return err
}

func prev(g *ui.Gui, v *ui.View) error {
	cx, cy := v.Cursor()
	err := v.SetCursor(cx, cy-1)
	printHosts()
	return err
}

func ssh(g *ui.Gui, v *ui.View) error {
	_, cy := v.Cursor()
	hosts[cy].selected = true
	return ui.ErrQuit
}

func sel(g *ui.Gui, v *ui.View) error {
	_, cy := v.Cursor()
	if hosts[cy].selected {
		hosts[cy].selected = false
	} else {
		hosts[cy].selected = true
	}
	printHosts()
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

// func getHosts() []string {
// 	f := []string{"p1", "s1"}
// 	c := []string{"a", "b", "c", "d", "e"}
// 	out := make([]string, len(c))
// 	for i, x := range c {
// 		out[i] = fmt.Sprintf("%s%s", strings.Repeat(x, 10), f[i%2])
// 	}
// 	return out
// }

func getHosts() []string {

	c, err := chef.Connect()
	if err != nil {
		log.Fatal("Error:", err)
	}
	c.SSLNoVerify = true

	q := fmt.Sprintf("hostname:*%s*", *query)
	if *role != "" {
		q = fmt.Sprintf("%s AND role:*%s*", q, *role)
	}

	resp, err := c.Search("node", q)
	if err != nil {
		log.Fatal("search", err)
	}

	var hosts []string
	for _, x := range resp.Rows {
		var n chef.Node
		json.Unmarshal(x, &n)
		hosts = append(hosts, n.Name)
	}
	return hosts
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
