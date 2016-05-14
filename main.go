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
	query    = kingpin.Arg("query", "search string").String()
	addr     = os.Getenv("UPTIME_ADDR")
	username = os.Getenv("UPTIME_USER")
	key      = os.Getenv("UPTIME_KEY")
)

type result struct {
	Host string `json:"fqdn"`
}

type resp struct {
	Message string   `json:"message"`
	Results []result `json:"results"`
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
		rows[i] = fmt.Sprintf("[%d] %s", i+1, x.Host)
	}
	return rows
}

func showResults(rows []string) string {
	//origRows := rows

	err := ui.Init()
	if err != nil {
		panic(err)
	}
	defer ui.Close()

	searchText := []string{""}
	ls, f := getLists(rows, searchText)

	ui.Render(ls, f)

	var result string
	var searchMode, confirmMode bool
	var indexStr string

	ui.Handle("/sys/kbd/C-d", func(ui.Event) {
		ui.StopLoop()
	})

	ui.Handle("/sys/kbd/C-f", func(ui.Event) {
		searchMode = true
		f.BorderLabel = "Filter (enter when done)"
		ui.Render(ls, f)
	})

	ui.Handle("/sys/kbd", func(e ui.Event) {
		if searchMode {
			s := getString(e.Data)
			if s == "C-8" {
				end := len(searchText[0]) - 1
				if end < 0 {
					end = 0
				}
				searchText[0] = searchText[0][0:end]
			} else if s == "<enter>" {
				searchMode = false
				f.BorderLabel = "Filter (C-f)"
			} else {
				searchText[0] += s
			}
			if len(searchText[0]) > 0 {
				ls.Items = filter(rows, searchText[0])
			} else {
				ls.Items = rows
			}
			ui.Render(ls, f)
		} else if confirmMode {
			s := getString(e.Data)
			if s == "y" {
				ui.StopLoop()
				return
			} else {
				result = ""
				ui.Clear()
				ui.Render(ls, f)
			}
		} else {
			s := getString(e.Data)
			if s == "<enter>" {
				j, err := strconv.ParseInt(indexStr, 10, 64)
				if err != nil {
					//alert
					return
				}
				i := int(j)
				if i <= len(rows) && i > 0 {
					result = rows[i-1]
					confirmMode = true
					c := getConfrm(result)
					indexStr = ""
					ui.Clear()
					ui.Render(c)
				}
			} else {
				indexStr += getString(e.Data)
			}
		}
	})

	ui.Loop()
	return result
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

func getLists(rows []string, searchText []string) (*ui.List, *ui.List) {
	width := getWidth(rows)

	ls := ui.NewList()
	ls.Items = rows
	ls.ItemFgColor = ui.ColorYellow
	ls.BorderLabel = "Results (C-d to exit)"
	ls.Height = len(rows) + 2
	ls.Width = width + 4
	ls.Y = 0

	f := ui.NewList()
	f.Items = searchText
	f.ItemFgColor = ui.ColorRed
	f.BorderLabel = "Filter (C-f)"
	f.Height = 3
	f.Width = width + 4
	f.Y = len(rows) + 2
	return ls, f
}

func filter(rows []string, sep string) []string {
	var out []string
	for _, r := range rows {
		if strings.Index(r, sep) > -1 {
			out = append(out, r)
		}
	}
	return out
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
