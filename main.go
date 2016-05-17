package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"

	ui "github.com/gizak/termui"

	"gopkg.in/alecthomas/kingpin.v2"
)

var (
	query     = kingpin.Arg("query", "search string").String()
	addr      = os.Getenv("UPTIME_ADDR")
	username  = os.Getenv("UPTIME_USER")
	key       = os.Getenv("UPTIME_KEY")
	userTheme theme
	themes    = map[string]theme{
		"light": {
			list:   ui.ColorBlack,
			filter: ui.ColorBlack,
			ssh:    ui.ColorBlack,
		},
		"dark": {
			list:   ui.ColorYellow,
			filter: ui.ColorGreen,
			ssh:    ui.ColorGreen,
		},
	}
	mode     int
	lastMode int
)

var helpStr = `Enter the number of the host(s) you want to ssh to then press enter.

Filter the list down by typeing C-f then type characters that appear in all the hosts you are searching for.  When you are done filtering hit enter.

You can select multiple hosts in two ways.  One is to use the filter to narrow down results then press C-a to start a csshx sesssion.  The other way is to type the number of each host separated by commas.  Then press enter to cssh to those hosts.

C-d to exit without sshing to anything.

Type q to exit this help.
`

const (
	modeDefault int = iota
	modeSearch
	modeConfirm
	modeHelp
)

type theme struct {
	list   ui.Attribute
	filter ui.Attribute
	ssh    ui.Attribute
}

type result struct {
	Host string `json:"fqdn"`
}

type uptime struct {
	Message string   `json:"message"`
	Results []result `json:"results"`
}

type lists struct {
	hosts   []string
	list    *ui.List
	filter  *ui.List
	ssh     *ui.List
	search  string
	index   string
	targets []string
}

func init() {
	var ok bool
	userTheme, ok = themes[os.Getenv("UPTIME_THEME")]
	if !ok {
		userTheme = themes["dark"]
	}
}

func main() {
	kingpin.Parse()
	hosts := getHosts()
	targets := getTargets(hosts)
	login(targets)
}

func getTargets(hosts []string) []string {
	err := ui.Init()
	if err != nil {
		log.Fatal(err)
	}
	defer ui.Close()

	ls := getLists(hosts)
	help := getHelp()
	ls.showNumbers(ls.hosts)

	ui.Render(ls.list, ls.filter, ls.ssh)

	//exit
	ui.Handle("/sys/kbd/C-d", func(ui.Event) {
		ls.targets = []string{""}
		ui.StopLoop()
	})

	//enter search mode
	ui.Handle("/sys/kbd/C-f", func(ui.Event) {
		mode = modeSearch
		ls.filter.BorderLabel = "Filter (enter when done)"
		ui.Render(ls.list, ls.filter, ls.ssh)
	})

	//help
	ui.Handle("/sys/kbd/<backspace>", func(ui.Event) {
		lastMode = mode
		mode = modeHelp
		ui.Clear()
		ui.Render(help)
	})

	//cssh to all visible hosts
	ui.Handle("/sys/kbd/C-a", func(ui.Event) {
		out := make([]string, len(ls.list.Items))
		for i, x := range ls.list.Items {
			parts := strings.Split(x, " ")
			out[i] = parts[len(parts)-1]
		}
		ls.targets = out
		ui.StopLoop()
	})

	//handle input
	ui.Handle("/sys/kbd", func(e ui.Event) {
		s := getString(e.Data)
		switch mode {
		case modeHelp:
			mode = lastMode
			ui.Clear()
			ui.Render(ls.list, ls.filter, ls.ssh)
		case modeSearch:
			ls.doFilter(s)
			ui.Render(ls.list, ls.filter, ls.ssh)
		default:
			if ls.getResult(s) {
				ui.StopLoop()
			}
		}
	})
	ui.Loop()
	return ls.targets
}

func (l *lists) getResult(in string) bool {
	switch in {
	case "<enter>":
		return true
	case "C-8":
		l.index = ""
		l.targets = []string{""}
		l.ssh.Items = []string{""}
		l.ssh.Height = 3
		l.ssh.BorderLabel = "ssh to (enter number)"
		ui.Clear()
	case ",":
		l.index = ""
		l.targets = append(l.targets, "")
		l.ssh.Items = append(l.ssh.Items, "")
		l.ssh.Height = l.ssh.Height + 1
	default:
		s := getString(in)
		parts := strings.Split(s, ",")
		index := l.index + parts[len(parts)-1]
		j, err := strconv.ParseInt(index, 10, 64)
		if err != nil {
			return false
		}
		i := int(j)
		if i > len(l.list.Items) || i <= 0 {
			return false
		}
		l.index = index
		i--
		r := l.list.Items[i]
		parts = strings.Split(r, " ")
		l.targets[len(l.targets)-1] = parts[len(parts)-1]
		l.ssh.Items[len(l.ssh.Items)-1] = l.targets[len(l.targets)-1]
		l.ssh.BorderLabel = fmt.Sprintf("ssh to %s (enter number)", l.index)
	}
	ui.Render(l.list, l.filter, l.ssh)
	return false
}

func (l *lists) delete() {
	end := len(l.search) - 1
	if end < 0 {
		end = 0
	}
	l.search = l.search[0:end]
}

func (l *lists) enter() {
	mode = modeDefault
	l.filter.BorderLabel = "Filter (C-f)"
}

func (l *lists) doFilter(s string) {
	switch s {
	case "C-8":
		l.delete()
	case "<enter>":
		l.enter()
	default:
		l.search += s
	}
	l.filter.Items = []string{l.search}
	var out []string
	for _, h := range l.hosts {
		if strings.Index(h, l.search) > -1 {
			out = append(out, h)
		}
	}
	l.showNumbers(out)
}

func getString(d interface{}) string {
	k := strings.Replace(fmt.Sprintf("%s", d), "{", "", -1)
	return strings.Replace(k, "}", "", -1)
}

func getWidth(hosts []string, label string) int {
	max := 0
	for _, h := range hosts {
		if len(h) > max {
			max = len(h)
		}
	}
	if len(label) > max {
		return len(label) + 3
	}
	return max + 7
}

func getIndex(d interface{}) (int, error) {
	k := strings.Replace(fmt.Sprintf("%s", d), "{", "", -1)
	k = strings.Replace(k, "}", "", -1)
	i, err := strconv.ParseInt(k, 10, 64)
	return int(i), err
}

func (l *lists) showNumbers(hosts []string) {
	out := make([]string, len(hosts))
	for i, r := range hosts {
		out[i] = fmt.Sprintf("%-2d %s", i+1, r)
	}
	l.list.Items = out
}

func getHelp() *ui.Par {
	par := ui.NewPar(helpStr)
	par.Height = 15
	par.Width = 80
	par.Y = 1
	par.Border = true
	return par
}

func getLists(hosts []string) *lists {
	label := "results (C-d to exit, C-a to csshx to all)"
	width := getWidth(hosts, label)

	h := len(hosts) + 2
	if h > 50 {
		h = 50
	}

	ls := ui.NewList()
	ls.Items = hosts
	ls.ItemFgColor = userTheme.list
	ls.BorderLabel = label
	ls.Height = h
	ls.Width = width
	ls.Y = 0

	f := ui.NewList()
	f.Items = []string{""}
	f.ItemFgColor = userTheme.filter
	f.BorderLabel = "filter (C-f)"
	f.Height = 3
	f.Width = width
	f.Y = h + 2

	s := ui.NewList()
	s.Items = []string{""}
	s.ItemFgColor = userTheme.ssh
	s.BorderLabel = "ssh to (enter number)"
	s.Height = 3
	s.Width = width
	s.Y = h + 5

	return &lists{hosts: hosts, list: ls, filter: f, ssh: s, targets: []string{""}}
}

func getConfrm(result string) *ui.List {
	parts := strings.Split(result, " ")
	label := fmt.Sprintf("Really ssh to %s (y/n)?", parts[1])
	c := ui.NewList()
	c.Items = []string{}
	c.ItemFgColor = ui.ColorYellow
	c.BorderLabel = label
	c.Height = 3
	c.Width = len(label) + 4
	c.Y = 0
	return c
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
	resp, err := http.Get(fmt.Sprintf("%s/servers/search/%s/%s", addr, q, key))
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
	return hosts
}
