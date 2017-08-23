package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/atotto/clipboard"
	ui "github.com/jroimartin/gocui"
	chef "github.com/marpaia/chef-golang"
	"gopkg.in/alecthomas/kingpin.v2"
)

const (
	forward  = iota
	backward = iota
	elipsis  = "..."
)

var (
	g            *ui.Gui
	query        = kingpin.Arg("query", "turns the arg into a knife search of 'hostname:<ARG>'").String()
	knife        = kingpin.Flag("knife", "uses the passed in value as a raw knife search").Short('k').String()
	filterStr    = kingpin.Flag("filter", "filter string").Short('f').String()
	fake         = kingpin.Flag("mock", "fake nodes").Short('m').Bool()
	role         = kingpin.Flag("role", "chef role").Short('r').String()
	scp          = kingpin.Flag("scp", "file to scp to targets").Short('s').String()
	username     string
	info         bool
	current      string
	hosts        []node
	visibleNodes []node
	chars        = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ1234567890.-,"
	colors       map[string]func(io.Writer, string)
	hostLabel    string
	filterLabel  string
	window       int
	f            *os.File
	msg          chan string
)

type node struct {
	node     chef.Node
	selected bool
	index    int
}

type byHost []node

func (b byHost) Len() int           { return len(b) }
func (b byHost) Swap(i, j int)      { b[i], b[j] = b[j], b[i] }
func (b byHost) Less(i, j int) bool { return b[i].node.Name < b[j].node.Name }

func init() {
	msg = make(chan string)
	f, _ = os.Create("/tmp/ussh.log")
	log.SetOutput(f)
	username = os.Getenv("USSH_USER")
	if username == "" {
		fmt.Println("please set the $USSH_USER env var to your ldap username.")
		os.Exit(1)
	}

	window = 20
	w := os.Getenv("USSH_WINDOW")
	if w != "" {
		x, err := strconv.ParseInt(w, 10, 32)
		if err == nil {
			window = int(x)
		}
	}
	setupColors()
}

func main() {
	kingpin.Parse()
	if *fake {
		getFakeNodes()
	} else {
		getNodes()
	}

	if *filterStr != "" {
		search(*filterStr)
	}

	targets := getTargets()
	f.Close()
	login(targets)
}

func getTargets() []string {
	g = ui.NewGui()
	if err := g.Init(); err != nil {
		log.Fatal("could not init", err)
	}

	go message()

	current = "hosts-cursor"

	g.Editor = ui.EditorFunc(edit)

	defer g.Close()

	if err := keybindings(g); err != nil {
		log.Panicln(err)
	}

	g.SetLayout(layout)
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

func search(pred string) {
	my := len(hosts)
	if g != nil {
		_, my = g.Size()
		my -= 5
	}
	pred = strings.TrimSpace(pred)
	preds := strings.Split(pred, ",")

	var i int
	visibleNodes = []node{}
	for _, n := range hosts {
		if i < window && inAll(n.node.Name, preds) {
			visibleNodes = append(visibleNodes, n)
			i++
		}
		if i >= window {
			break
		}
	}
}

func inAll(h string, preds []string) bool {
	for _, p := range preds {
		if strings.Index(h, p) == -1 {
			return false
		}
	}
	return true
}

func getWidth() int {
	var w int
	for _, n := range hosts {
		if len(n.node.Name) > w {
			w = len(n.node.Name)
		}
	}
	return w
}

func showHelp(g *ui.Gui, v *ui.View) error {
	x, y := g.Size()
	if v, err := g.SetView("help", -1, -1, x, y); err != nil {
		if err != ui.ErrUnknownView {
			return err
		}
		f := colors["color2"]
		f(v, "  help\n")
		f(v, "	   C-f: Enter filter mode (hit enter to exit filter mode)")
		f(v, "	   C-a: Csshx to all visible hosts")
		f(v, "	   Enter: Ssh to the highlighted host(s)")
		f(v, "	   Space: Toggle select the host on the current line")
		f(v, "	   C-i: Show info for the current host")
		f(v, "	   C-d: Quit without sshing")
		f(v, "	   n: Move cursor to the next host (down arrow does same thing)")
		f(v, "	   p: Move cursor to the previous host (up arrow does same thing)")
		f(v, "	   c: Copy the current host to the clipboard")
		f(v, "	   C: Copy the current host to the clipboard with 'USSH_USER@' prepended to the host")
		f(v, "	   q: Exit the help screen")
		current = "help"
		v.Editable = false
	}
	return g.SetCurrentView("help")
}

func exitHelp(g *ui.Gui, v *ui.View) error {
	if current != "help" {
		return nil
	}
	current = "hosts-cursor"
	return g.DeleteView("help")
}

func layout(g *ui.Gui) error {
	x, y := g.Size()
	size := len(visibleNodes)
	width := getWidth()

	if v, err := g.SetView("hosts-label", -1, -1, len("hosts"), 1); err != nil {
		if err != ui.ErrUnknownView {
			return err
		}
		v.Frame = false
		fmt.Fprintln(v, hostLabel)
	}

	if v, err := g.SetView("msg", len("hosts")+1, -1, x, 1); err != nil {
		if err != ui.ErrUnknownView {
			return err
		}
		v.Highlight = false
		v.Frame = false
		v.Editable = false
		colors["color3"](v, "")
	}

	if v, err := g.SetView("hosts-cursor", 4, 0, 6, size+1); err != nil {
		if err != ui.ErrUnknownView {
			return err
		}
		v.Highlight = true
		v.Frame = false
	}

	if v, err := g.SetView("hosts", 6, 0, width+11, size+1); err != nil {
		if err != ui.ErrUnknownView {
			return err
		}
		v.Frame = false
		printNodes()
	}

	if v, err := g.SetView("filter-label", -1, size, width+13, size+2); err != nil {
		if err != ui.ErrUnknownView {
			return err
		}
		v.FgColor = ui.ColorGreen
		v.Highlight = false
		v.Frame = false
		v.Editable = false
		fmt.Fprintln(v, filterLabel)
	}

	if v, err := g.SetView("filter", 4, size+1, width, size+3); err != nil {
		if err != ui.ErrUnknownView {
			return err
		}
		v.FgColor = ui.ColorGreen
		v.Highlight = false
		v.Frame = false
		v.Editable = true
		search(*filterStr)
		fmt.Fprintln(v, *filterStr)
		*filterStr = ""
	}

	if v, err := g.SetView("info", width+13, 0, x, y-1); err != nil {
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
		prefix, postfix := getElipsis(i)
		f := colors["color1"]
		if n.selected && i == cur {
			f = colors["color3"]
		} else if n.selected || i == cur {
			f = colors["color2"]
		}
		f(hv, fmt.Sprintf("%s%s%s", prefix, n.node.Name, postfix))
	}
}

func getElipsis(i int) (string, string) {
	if len(visibleNodes) < window {
		return "", ""
	}
	if i == 0 && visibleNodes[0].index > 0 {
		return elipsis, ""
	}
	if i == window-1 && (visibleNodes[window-1].index != hosts[len(hosts)-1].index) {
		return "", elipsis
	}
	return "", ""
}

func edit(v *ui.View, key ui.Key, ch rune, mod ui.Modifier) {
	if key == ui.KeyEnter {
		cv, _ := g.View("hosts-cursor")
		cv.SetCursor(0, 0)
		current = "hosts-cursor"
		printNodes()
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
	visibleNodes = make([]node, len(hosts))
	copy(visibleNodes, hosts)
}

func acceptable(s string) bool {
	return strings.Contains(chars, s)
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
	{"", 'q', ui.ModNone, exitHelp},
	{"filter", ui.KeyEnter, ui.ModNone, exitFilter},
	{"hosts-cursor", ui.KeyCtrlN, ui.ModNone, next},
	{"hosts-cursor", 'n', ui.ModNone, next},
	{"hosts-cursor", ui.KeyArrowDown, ui.ModNone, next},
	{"hosts-cursor", ui.KeyCtrlP, ui.ModNone, prev},
	{"hosts-cursor", 'p', ui.ModNone, prev},
	{"hosts-cursor", ui.KeyArrowUp, ui.ModNone, prev},
	{"hosts-cursor", ui.KeySpace, ui.ModNone, sel},
	{"hosts-cursor", ui.KeyEnter, ui.ModNone, ssh},
	{"hosts-cursor", ui.KeyCtrlI, ui.ModNone, showInfo},
	{"hosts-cursor", 'i', ui.ModNone, showInfo},
	{"hosts-cursor", ui.KeyCtrlF, ui.ModNone, filter},
	{"hosts-cursor", 'f', ui.ModNone, filter},
	{"hosts-cursor", ui.KeyCtrlH, ui.ModNone, showHelp},
	{"hosts-cursor", 'h', ui.ModNone, showHelp},
	{"hosts-cursor", ui.KeyCtrlC, ui.ModNone, copyToClipboard},
	{"hosts-cursor", 'c', ui.ModNone, copyToClipboard},
	{"hosts-cursor", 'C', ui.ModNone, copyToClipboardWithUsername},
}

func keybindings(g *ui.Gui) error {
	for _, k := range keys {
		if err := g.SetKeybinding(k.name, k.key, k.mod, k.f); err != nil {
			return err
		}
	}
	return nil
}

func quit(g *ui.Gui, v *ui.View) error {
	return ui.ErrQuit
}

func copyToClipboard(g *ui.Gui, v *ui.View) error {
	cv, _ := g.View("hosts-cursor")
	_, cur := cv.Cursor()
	n := visibleNodes[cur]
	msg <- fmt.Sprintf("copied %s to clipboard", n.node.Name)
	return clipboard.WriteAll(n.node.Name)
}

func copyToClipboardWithUsername(g *ui.Gui, v *ui.View) error {
	cv, _ := g.View("hosts-cursor")
	_, cur := cv.Cursor()
	n := visibleNodes[cur]
	s := fmt.Sprintf("%s@%s", username, n.node.Name)
	msg <- fmt.Sprintf("copied %s to clipboard", s)
	return clipboard.WriteAll(s)
}

func message() {
	dur := time.Second * 2
	for {
		select {
		case m := <-msg:
			dur = time.Second * 2
			writeMsg(m)
		case <-time.After(dur):
			dur = time.Second * 1000
			writeMsg("")
		}
	}
}

func writeMsg(msg string) {
	g.Execute(func(g *ui.Gui) error {
		v, _ := g.View("msg")
		v.Clear()
		fmt.Fprint(v, msg)
		return nil
	})
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

func exitFilter(g *ui.Gui, v *ui.View) error {
	current = "hosts-cursor"
	return nil
}

func sshAll(g *ui.Gui, v *ui.View) error {
	for i := range visibleNodes {
		visibleNodes[i].selected = true
	}
	return ui.ErrQuit
}

func scroll(dir int) {
	if len(visibleNodes) < window {
		return
	}
	if dir == forward {
		if visibleNodes[window-1].index == len(hosts)-1 {
			return
		}
		start := visibleNodes[0].index + 1
		end := start + window
		visibleNodes = hosts[start:end]
	} else {
		if visibleNodes[0].index == 0 {
			return
		}
		start := visibleNodes[0].index - 1
		end := start + window
		visibleNodes = hosts[start:end]
	}
	printNodes()
}

func next(g *ui.Gui, v *ui.View) error {
	cx, cy := v.Cursor()
	if cy+1 >= len(visibleNodes) {
		scroll(forward)
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
		scroll(backward)
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
		fmt.Fprintf(v, "Roles: %v\n", n.node.Info.Roles)
		fmt.Fprintf(v, "Environment: %s\n", n.node.Environment)
		fmt.Fprintf(v, "IP: %s\n", n.node.Info.IPAddress)
		fmt.Fprintf(v, "MAC: %s\n", n.node.Info.MACAddress)
		fmt.Fprintf(v, "Uptime: %s\n", n.node.Info.Uptime)
		printMemory(v, n.node)
		printFilesystem(v, n.node)
		printCPU(v, n.node)
	}
}

func printMemory(v *ui.View, n chef.Node) {
	fmt.Fprint(v, "Memory:\n")
	fmt.Fprintf(v, "  active: %s\n", n.Info.Memory["active"])
	fmt.Fprintf(v, "  free: %s\n", n.Info.Memory["free"])
	fmt.Fprintf(v, "  inactive: %s\n", n.Info.Memory["inactive"])
}

func printFilesystem(v *ui.View, n chef.Node) {
	fmt.Fprint(v, "Filesystem:\n")
	for k, val := range n.Info.Filesystem {
		if strings.Index(k, "/dev") > -1 {
			fmt.Fprintf(v, "  %s\n", k)
			fmt.Fprintf(v, "    Size: %v\n", val.KBSize)
			fmt.Fprintf(v, "    Used: %v\n", val.KBUsed)
			fmt.Fprintf(v, "    Available: %v\n", val.KBavailable)
			fmt.Fprintf(v, "    PercentUsed: %v\n", val.PercentUsed)
		}
	}
}

func printCPU(v *ui.View, n chef.Node) {
	fmt.Fprint(v, "CPU: \n")
	for k, val := range n.Info.CPU {
		if k == "cores" {
			fmt.Fprintf(v, "  %s\n", k)
			fmt.Fprintf(v, "    Cores: %v\n", val)
		}

		if k == "cores" {
			fmt.Fprintf(v, "    Cores: %v\n", val)
		}
	}
}

func login(targets []string) {
	if len(targets) == 1 && targets[0] != "" && len(*scp) == 0 {
		cmd := exec.Command("ssh", fmt.Sprintf("%s@%s", username, targets[0]))
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		err := cmd.Run()
		if err != nil {
			log.Fatal("couldn't ssh", err)
		}
	} else if len(targets) >= 1 && targets[0] != "" && len(*scp) > 0 {
		for _, x := range targets {
			cmd := exec.Command("scp", *scp, fmt.Sprintf("%s@%s:", username, x))
			cmd.Stdin = os.Stdin
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			err := cmd.Run()
			if err != nil {
				log.Fatal("couldn't scp", err)
			}
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
	f := []string{"com", "net"}
	e := []string{"prod", "staging"}
	hosts = make([]node, 30)
	for i := 0; i < 30; i++ {
		n := node{node: chef.Node{Name: fmt.Sprintf("server%d.%s", i, f[i%2]), Environment: e[i%2]}, index: i}
		hosts[i] = n
		if i < window {
			visibleNodes = append(visibleNodes, n)
		}
	}
}

func getNodes() {

	c, err := chef.Connect()
	if err != nil {
		log.Fatal("Error:", err)
	}

	c.SSLNoVerify = true

	var q string
	if *knife != "" {
		q = *knife
	} else {
		q = fmt.Sprintf("hostname:*%s*", *query)
		if *role != "" {
			q = fmt.Sprintf("%s AND role:*%s*", q, *role)
		}
	}

	resp, err := c.Search("node", q)
	if err != nil {
		log.Fatal("search", err)
	}

	if len(resp.Rows) == 0 {
		fmt.Printf("No nodes found with query %s, please try again with a different search\n", *query)
		os.Exit(0)
	}

	for i, x := range resp.Rows {
		var cn chef.Node
		json.Unmarshal(x, &cn)
		n := node{node: cn, index: i}
		hosts = append(hosts, n)
	}

	sort.Sort(byHost(hosts))
	for i, _ := range hosts {
		hosts[i].index = i
	}
	end := window
	if len(hosts) < window {
		end = len(hosts)
	}
	visibleNodes = hosts[0:end]

}

func setupColors() {
	m := map[string]string{
		"black":   "30",
		"red":     "31",
		"green":   "32",
		"yellow":  "33",
		"blue":    "34",
		"magenta": "35",
		"cyan":    "36",
		"white":   "37",
	}

	color1 := os.Getenv("USSH_COLOR_1")
	if color1 == "" {
		color1 = "green"
	}
	color2 := os.Getenv("USSH_COLOR_2")
	if color2 == "" {
		color2 = "white"
	}
	color3 := os.Getenv("USSH_COLOR_3")
	if color3 == "" {
		color3 = "yellow"
	}

	colors = map[string]func(io.Writer, string){
		"color1": func(w io.Writer, s string) {
			fmt.Fprintf(w, fmt.Sprintf("\033[%sm%%s\033[%sm\n", m[color1], m[color1]), s)
		},

		"color2": func(w io.Writer, s string) {
			fmt.Fprintf(w, fmt.Sprintf("\033[%sm%%s\033[%sm\n", m[color2], m[color1]), s)
		},

		"color3": func(w io.Writer, s string) {
			fmt.Fprintf(w, fmt.Sprintf("\033[%sm%%s\033[%sm\n", m[color3], m[color1]), s)
		},
	}

	hostLabel = fmt.Sprintf("\033[%smhosts\033[%sm\n", m[color2], m[color1])
	filterLabel = fmt.Sprintf("\033[%smfilter\033[%sm\n", m[color2], m[color1])
}
