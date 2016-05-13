package main

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"

	"golang.org/x/crypto/ssh"

	"github.com/gizak/termui"

	"gopkg.in/alecthomas/kingpin.v2"
)

//import "github.com/gizak/termui"

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
	login(parts[1])
}

func login(host string) {

	key, err := ioutil.ReadFile(fmt.Sprintf("%s/.ssh/id_rsa", os.Getenv("HOME")))
	if err != nil {
		log.Fatalf("unable to read private key: %v", err)
	}

	// Create the Signer for this private key.
	signer, err := ssh.ParsePrivateKey(key)
	if err != nil {
		log.Fatalf("unable to parse private key: %v", err)
	}

	config := &ssh.ClientConfig{
		User: username,
		Auth: []ssh.AuthMethod{
			// Use the PublicKeys method for remote authentication.
			ssh.PublicKeys(signer),
		},
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
	session.Stdin = os.Stdin

	if err := session.Shell(); err != nil {
		log.Fatal(err)
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
	width := getWidth(rows)
	err := termui.Init()
	if err != nil {
		panic(err)
	}
	defer termui.Close()

	ls := termui.NewList()
	ls.Items = rows
	ls.ItemFgColor = termui.ColorYellow
	ls.BorderLabel = "Results"
	ls.Height = len(rows) + 2
	ls.Width = width + 4
	ls.Y = 0

	var result string

	termui.Render(ls)

	termui.Handle("/sys/kbd/C-d", func(termui.Event) {
		termui.StopLoop()
	})

	termui.Handle("/sys/kbd", func(e termui.Event) {
		i, err := getIndex(e.Data)
		if err != nil {
			//alert
			return
		}
		termui.StopLoop()
		result = rows[i-1]
	})

	termui.Loop()
	return result
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

func getIndex(d interface{}) (int64, error) {
	k := strings.Replace(fmt.Sprintf("%s", d), "{", "", -1)
	k = strings.Replace(k, "}", "", -1)
	return strconv.ParseInt(k, 10, 64)
}
