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

type resultList struct {
	rows   []string
	filter string
	list   *ui.List
}

type filterList struct {
	sep  string
	rows []string
	list *ui.List
}

type sshList struct {
	rows   []string
	index  string
	list   *ui.List
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
	parts := strings.Split(s, " ")
	if len(parts) == 2 {
		login(parts[1])
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
	fmt.Println(rows)
	return rows
}

func showResults(rows []string) string {
	err := ui.Init()
	if err != nil {
		panic(err)
	}
	defer ui.Close()

	ls, f, t := getLists(rows)

	ui.Render(ls.list, f.list, t.list)

	ui.Handle("/sys/kbd/C-d", func(ui.Event) {
		t.target = ""
		ui.StopLoop()
	})

	ui.Handle("/sys/kbd/C-f", func(ui.Event) {
		mode = modeSearch
		f.list.BorderLabel = "Filter (enter when done)"
		ui.Render(ls.list, f.list, t.list)
	})

	ui.Handle("/sys/kbd", func(e ui.Event) {
		s := getString(e.Data)
		switch mode {
		case modeSearch:
			f.filter(s, ls)
		case modeConfirm:
			if s == "y" {
				ui.StopLoop()
				return
			}
			mode = modeDefault
			t.target = ""
			ui.Clear()
			ui.Render(ls.list, f.list, t.list)
		default:
			t.getResult(s)
			ui.Render(ls.list, f.list, t.list)
		}
	})
	ui.Loop()
	return t.target
}

func (s *sshList) getResult(in string) {
	switch in {
	case "<enter>":
		s.showConfirm()
	case "C-8":
		s.index = ""
		s.target = ""
		s.list.BorderLabel = "ssh to (enter number)"
		s.list.Items = []string{""}
	default:
		s.index += getString(in)
		j, err := strconv.ParseInt(s.index, 10, 64)
		if err != nil {
			return
		}
		i := int(j)
		if !(i <= len(s.rows) || i > 0) {
			return
		}
		s.target = s.rows[i]
		s.list.Items = []string{s.target}
		s.list.BorderLabel = fmt.Sprintf("ssh to %s (enter number)", s.index)
	}
}

func (s *sshList) showConfirm() {
	mode = modeConfirm
	label := fmt.Sprintf("Really ssh to %s (y/n)?", s.target)
	c := ui.NewList()
	c.Items = []string{}
	c.ItemFgColor = ui.ColorYellow
	c.BorderLabel = label
	c.Height = 3
	c.Width = len(label) + 4
	c.Y = 0
	ui.Clear()
	ui.Render(c)
}

func (f *filterList) delete() {
	end := len(f.sep) - 1
	if end < 0 {
		end = 0
	}
	f.sep = f.sep[0:end]
}

func (f *filterList) enter() {
	mode = modeDefault
	f.list.BorderLabel = "Filter (C-f)"
}

func (f *filterList) filter(s string, l *resultList) {
	switch s {
	case "C-8":
		f.delete()
	case "<ender>":
		f.enter()
	default:
		f.sep += s
	}
	var out []string
	for _, r := range f.rows {
		if strings.Index(r, f.sep) > -1 {
			out = append(out, r)
		}
	}
	l.list.Items = out
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

func getLists(rows []string) (*resultList, *filterList, *sshList) {
	width := getWidth(rows)

	ls := ui.NewList()
	ls.Items = rows
	ls.ItemFgColor = userTheme.list
	ls.BorderLabel = "sesults (C-d to exit)"
	ls.Height = len(rows) + 2
	ls.Width = width + 4
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

	return &resultList{rows: rows, list: ls}, &filterList{rows: rows, list: f}, &sshList{rows: rows, list: s}
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
