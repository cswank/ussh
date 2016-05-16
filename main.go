package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
	"golang.org/x/crypto/ssh/terminal"

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
	mode int
)

const (
	modeDefault int = iota
	modeSearch
	modeConfirm
)

type theme struct {
	list   ui.Attribute
	filter ui.Attribute
	ssh    ui.Attribute
}

type result struct {
	Host string `json:"fqdn"`
}

type resp struct {
	Message string   `json:"message"`
	Results []result `json:"results"`
}

type lists struct {
	rows   []string
	list   *ui.List
	filter *ui.List
	ssh    *ui.List
	search string
	index  string
	target string
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
	q := strings.Replace(*query, "%", "%25", -1)
	r, err := http.Get(fmt.Sprintf("%s/servers/search/%s/%s", addr, q, key))
	if err != nil {
		log.Fatal(err)
	}
	defer r.Body.Close()
	rows := getRows(r.Body)
	s := showResults(rows)
	if s != "" {
		login(s)
	}
}

func getRows(body io.Reader) []string {
	var r resp
	dec := json.NewDecoder(body)
	if err := dec.Decode(&r); err != nil {
		log.Fatal(err)
	}
	rows := make([]string, len(r.Results))

	for i, x := range r.Results {
		rows[i] = x.Host
	}
	return rows
}

func showResults(rows []string) string {
	err := ui.Init()
	if err != nil {
		panic(err)
	}
	defer ui.Close()

	ls := getLists(rows)
	ls.showNumbers(ls.rows)

	ui.Render(ls.list, ls.filter, ls.ssh)

	ui.Handle("/sys/kbd/C-d", func(ui.Event) {
		ls.target = ""
		ui.StopLoop()
	})

	ui.Handle("/sys/kbd/C-f", func(ui.Event) {
		mode = modeSearch
		ls.filter.BorderLabel = "Filter (enter when done)"
		ui.Render(ls.list, ls.filter, ls.ssh)
	})

	ui.Handle("/sys/kbd", func(e ui.Event) {
		s := getString(e.Data)
		switch mode {
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
	return ls.target
}

func (l *lists) getResult(in string) bool {
	switch in {
	case "<enter>":
		return true
	case "C-8":
		l.index = ""
		l.target = ""
		l.ssh.Items = []string{""}
		l.ssh.BorderLabel = "ssh to (enter number)"
		ui.Render(l.list, l.filter, l.ssh)
	default:
		index := l.index + getString(in)
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
		parts := strings.Split(r, " ")
		l.target = parts[len(parts)-1]
		l.ssh.Items = []string{l.target}
		l.ssh.BorderLabel = fmt.Sprintf("ssh to %s (enter number)", l.index)
		ui.Render(l.list, l.filter, l.ssh)
	}
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
	for _, r := range l.rows {
		if strings.Index(r, l.search) > -1 {
			out = append(out, r)
		}
	}
	l.showNumbers(out)
}

func getString(d interface{}) string {
	k := strings.Replace(fmt.Sprintf("%s", d), "{", "", -1)
	return strings.Replace(k, "}", "", -1)
}

func getWidth(rows []string) int {
	max := 0
	for _, r := range rows {
		if len(r) > max {
			max = len(r)
		}
	}
	return max
}

func getIndex(d interface{}) (int, error) {
	k := strings.Replace(fmt.Sprintf("%s", d), "{", "", -1)
	k = strings.Replace(k, "}", "", -1)
	i, err := strconv.ParseInt(k, 10, 64)
	return int(i), err
}

func login(host string) {

	conn, err := net.Dial("unix", os.Getenv("SSH_AUTH_SOCK"))
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close()
	ag := agent.NewClient(conn)
	auths := []ssh.AuthMethod{ssh.PublicKeysCallback(ag.Signers)}

	config := &ssh.ClientConfig{
		User: username,
		Auth: auths,
	}

	// Connect to the remote server and perform the SSH handshake.
	client, err := ssh.Dial("tcp", fmt.Sprintf("%s:22", host), config)
	if err != nil {
		log.Fatalf("unable to connect: %v", err)
	}
	defer client.Close()

	session, err := client.NewSession()
	if err != nil {
		panic("Failed to create session: " + err.Error())
	}
	defer session.Close()

	session.Stdout = os.Stdout
	session.Stderr = os.Stderr
	session.Stdin = os.Stdin

	modes := ssh.TerminalModes{
		ssh.ECHO:          1,     // enable echoing
		ssh.TTY_OP_ISPEED: 14400, // input speed = 14.4kbaud
		ssh.TTY_OP_OSPEED: 14400, // output speed = 14.4kbaud
	}

	fileDescriptor := int(os.Stdin.Fd())

	if terminal.IsTerminal(fileDescriptor) {
		originalState, err := terminal.MakeRaw(fileDescriptor)
		if err != nil {
			log.Fatal(err)
		}
		defer terminal.Restore(fileDescriptor, originalState)

		termWidth, termHeight, err := terminal.GetSize(fileDescriptor)
		if err != nil {
			log.Fatal(err)
		}

		err = session.RequestPty("xterm-256color", termHeight, termWidth, modes)
		if err != nil {
			log.Fatal(err)
		}
	}

	err = session.Shell()
	if err != nil {
		log.Fatal(err)
	}

	session.Wait()
}

func (l *lists) showNumbers(rows []string) {
	out := make([]string, len(rows))
	for i, r := range rows {
		out[i] = fmt.Sprintf("%-2d %s", i+1, r)
	}
	l.list.Items = out
}

func getLists(rows []string) *lists {
	width := getWidth(rows)

	ls := ui.NewList()
	ls.Items = rows
	ls.ItemFgColor = userTheme.list
	ls.BorderLabel = "sesults (C-d to exit)"
	ls.Height = len(rows) + 2
	ls.Width = width + 7
	ls.Y = 0

	f := ui.NewList()
	f.Items = []string{""}
	f.ItemFgColor = userTheme.filter
	f.BorderLabel = "filter (C-f)"
	f.Height = 3
	f.Width = width + 4
	f.Y = len(rows) + 2

	s := ui.NewList()
	s.Items = []string{""}
	s.ItemFgColor = userTheme.ssh
	s.BorderLabel = "ssh to (enter number)"
	s.Height = 3
	s.Width = width + 4
	s.Y = len(rows) + 5

	return &lists{rows: rows, list: ls, filter: f, ssh: s}
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
