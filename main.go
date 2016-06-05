package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"strings"

	ui "github.com/jroimartin/gocui"
	chef "github.com/marpaia/chef-golang"
	"gopkg.in/alecthomas/kingpin.v2"
)

var (
	g            *ui.Gui
	query        = kingpin.Arg("query", "search string").String()
	fake         = kingpin.Flag("fake", "fake nodes").Short('f').Bool()
	role         = kingpin.Flag("role", "chef role").Short('r').String()
	addr         = os.Getenv("UPTIME_ADDR")
	username     = os.Getenv("UPTIME_USER")
	secret       = os.Getenv("UPTIME_KEY")
	info         bool
	current      string
	nodes        []node
	visibleNodes []node
	chars        = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ1234567890.-,"
	colors       = map[string]func(io.Writer, string){
		"green":  func(w io.Writer, s string) { fmt.Fprintf(w, "\033[32m%s\033[37m\n", s) },
		"white":  func(w io.Writer, s string) { fmt.Fprintf(w, "\033[37m%s\033[37m\n", s) },
		"yellow": func(w io.Writer, s string) { fmt.Fprintf(w, "\033[33m%s\033[37m\n", s) },
	}
)

type node struct {
	node     chef.Node
	selected bool
}

func init() {
	f, _ := os.Create("/tmp/ussh")
	log.SetOutput(f)
}

func main() {
	kingpin.Parse()
	if *fake {
		getFakeNodes()
	} else {
		getNodes()
	}
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
	visibleNodes = []node{}
	for i, n := range nodes {
		if i < my && inBoth(n.node.Name, preds) {
			visibleNodes = append(visibleNodes, n)
		}
	}
}

func getTargets() []string {
	g = ui.NewGui()
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

	if err := g.MainLoop(); err != nil {
		if err != ui.ErrQuit {
			log.Fatal(err)
		}
	}
	var out []string
	for _, n := range visibleNodes {
		if n.selected {
			out = append(out, n.node.Name)
		}
	}
	return out
}

func getWidth() int {
	var w int
	for _, n := range nodes {
		if len(n.node.Name) > w {
			w = len(n.node.Name)
		}
	}
	return w
}

func layout(g *ui.Gui) error {
	x, _ := g.Size()
	size := len(visibleNodes)
	width := getWidth()

	if v, err := g.SetView("hosts-label", -1, -1, width+10, 1); err != nil {
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
	}

	if v, err := g.SetView("hosts", 6, 0, width+8, size+1); err != nil {
		if err != ui.ErrUnknownView {
			return err
		}
		v.Frame = false
		printNodes()
	}

	if v, err := g.SetView("filter-label", -1, size, width+10, size+2); err != nil {
		if err != ui.ErrUnknownView {
			return err
		}
		v.FgColor = ui.ColorGreen
		v.Highlight = false
		v.Frame = false
		v.Editable = false
		fmt.Fprintln(v, "filter: ")
	}

	if v, err := g.SetView("filter", 4, size+1, width, size+3); err != nil {
		if err != ui.ErrUnknownView {
			return err
		}
		v.FgColor = ui.ColorGreen
		v.Highlight = false
		v.Frame = false
		v.Editable = true
	}

	if v, err := g.SetView("info", width+10, 0, x, 10); err != nil {
		if err != ui.ErrUnknownView {
			return err
		}
		v.FgColor = ui.ColorGreen
		v.Highlight = false
		v.Frame = false
		v.Editable = false
		printInfo(v)
	}

	return g.SetCurrentView(current)
}

func printNodes() {
	hv, _ := g.View("hosts")
	cv, _ := g.View("hosts-cursor")
	hv.Clear()
	_, cur := cv.Cursor()
	for i, n := range visibleNodes {
		f := colors["green"]
		if n.selected && i == cur {
			f = colors["yellow"]
		} else if n.selected || i == cur {
			f = colors["white"]
		}
		f(hv, n.node.Name)
	}
}

func edit(v *ui.View, key ui.Key, ch rune, mod ui.Modifier) {
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
		unhideAll()
	} else if acceptable(string(ch)) {
		fmt.Fprint(v, string(ch))
		s = v.Buffer()
		search(s)
		v.SetCursor(len(s)-1, 0)
	}
	printNodes()
}

func unhideAll() {
	visibleNodes = make([]node, len(nodes))
	copy(visibleNodes, nodes)
}

func acceptable(s string) bool {
	return strings.Index(chars, s) > -1
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
	return v.SetCursor(l, 0)
}

func doFilter(g *ui.Gui, v *ui.View) error {
	current = "hosts-cursor"
	return nil
}

func sshAll(g *ui.Gui, v *ui.View) error {
	for i := range visibleNodes {
		visibleNodes[i].selected = true
	}
	return ui.ErrQuit
}

func next(g *ui.Gui, v *ui.View) error {
	cx, cy := v.Cursor()
	if cy+1 >= len(visibleNodes) {
		return nil
	}
	err := v.SetCursor(cx, cy+1)
	printNodes()
	iv, _ := g.View("info")
	printInfo(iv)
	return err
}

func prev(g *ui.Gui, v *ui.View) error {
	cx, cy := v.Cursor()
	if cy-1 < 0 {
		return nil
	}
	err := v.SetCursor(cx, cy-1)
	printNodes()
	iv, _ := g.View("info")
	printInfo(iv)
	return err
}

func ssh(g *ui.Gui, v *ui.View) error {
	_, cy := v.Cursor()
	visibleNodes[cy].selected = true
	return ui.ErrQuit
}

func sel(g *ui.Gui, v *ui.View) error {
	_, cy := v.Cursor()
	if visibleNodes[cy].selected {
		visibleNodes[cy].selected = false
	} else {
		visibleNodes[cy].selected = true
	}
	printNodes()
	return nil
}

func showInfo(g *ui.Gui, v *ui.View) error {
	v, _ = g.View("info")
	if info {
		v.Clear()
		info = false
	} else {
		info = true
		printInfo(v)
	}
	return nil
}

func printInfo(v *ui.View) {
	if info {
		v.Clear()
		cv, _ := g.View("hosts-cursor")
		_, cur := cv.Cursor()
		n := visibleNodes[cur]
		fmt.Fprintf(v, "Name: %s\n", n.node.Name)
		fmt.Fprintf(v, "Environment: %s\n", n.node.Environment)
	}
}

type key struct {
	name string
	key  interface{}
	mod  ui.Modifier
	f    ui.KeybindingHandler
}

var keys = []key{
	{"", ui.KeyCtrlC, ui.ModNone, quit},
	{"", ui.KeyCtrlI, ui.ModNone, showInfo},
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

func getFakeNodes() {
	f := []string{"p1", "s1"}
	c := []string{"a", "b", "c", "d", "e"}
	nodes = make([]node, len(c))
	visibleNodes = make([]node, len(c))
	for i, x := range c {
		n := node{node: chef.Node{Name: fmt.Sprintf("%s%s", strings.Repeat(x, 10), f[i%2])}}
		nodes[i] = n
		visibleNodes[i] = n
	}
}

func getNodes() {

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

	for _, x := range resp.Rows {
		var n chef.Node
		json.Unmarshal(x, &n)
		nodes = append(nodes, node{node: n})
		visibleNodes = append(visibleNodes, node{node: n})
	}
}
