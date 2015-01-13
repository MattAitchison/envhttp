package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
)

var (
	httpURL = flag.String("url", "", "URL to poll for env vars")
	headers = Header{}
	//accept   = flag.String("accept", "", "HTTP Header for Accepted media types")
	interval = flag.Duration("interval", 10*time.Second, "Interval in seconds to poll url")
	// TODO: Needs better comment
	useEtag = flag.Bool("etag", true, "Send last Etag to request changed data.")
)

// Header implements flag.Value
type Header struct {
	http.Header
}

// String is the method to format the flag's value, part of the flag.Value interface.
// The String method's output will be used in diagnostics.
func (h *Header) String() string {
	return fmt.Sprint(*h)
}

// Set is the method to parse and set a flag.Value
func (h *Header) Set(value string) error {
	if h.Header == nil {
		h.Header = http.Header{}
	}

	for _, dt := range strings.Split(value, ",") {

		splitText := strings.SplitN(dt, ":", 2)
		if len(splitText) != 2 {
			return errors.New("invalid format")
		}

		h.Header.Add(splitText[0], splitText[1])
	}
	return nil
}

type Runner struct {
	data map[string]string
	env  map[string]string
}

// parse data from io.Reader and return parsed env.
func parse(reader io.Reader) map[string]string {
	envMap := make(map[string]string)
	scanner := bufio.NewScanner(reader)

	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "#") {

			splitText := strings.SplitN(line, "=", 2)
			if len(splitText) == 2 {
				envMap[splitText[0]] = splitText[1]
			}
		}
	}
	return envMap
}

// Poller http poller to retrieve env
type Poller struct {
	DataCh      chan map[string]string
	ErrCh       chan error
	etagSupport bool
	lastEtag    string
	interval    time.Duration
	client      *http.Client
	headers     http.Header
	url         string
}

// NewPoller will return a poller with default config and everything initialized.
func NewPoller(url string, interval time.Duration, etagSupport bool, headers http.Header) *Poller {
	p := &Poller{
		interval:    interval,
		url:         url,
		etagSupport: etagSupport,
		client:      &http.Client{},
		headers:     headers,
	}

	p.DataCh = make(chan map[string]string)
	p.ErrCh = make(chan error)

	return p
}

func (p *Poller) poll() {
	log.Println("Polling...")

	req, err := http.NewRequest("GET", p.url, nil)
	if err != nil {
		p.ErrCh <- err
		return
	}
	if p.headers != nil {
		req.Header = p.headers
	}

	if p.etagSupport {
		req.Header.Set("If-None-Match", p.lastEtag)
	}

	log.Println("HTTP REQ: ", req)

	resp, err := p.client.Do(req)
	if err != nil {
		p.ErrCh <- err
		return
	}

	p.lastEtag = strings.Join(resp.Header["Etag"], "")

	defer resp.Body.Close()
	body := parse(resp.Body)
	p.DataCh <- body
}

// Run will start the Poller.
func (p *Poller) Run() {
	log.Println("Poller Started")
	for {
		go func() {
			p.poll()
		}()
		time.Sleep(p.interval)
	}

}

func main() {
	flag.Var(&headers, "header", "Headers")
	flag.Parse()

	if *httpURL == "" {
		log.Fatal("url is required")
	}

	log.Printf("%s", headers)

	p := NewPoller(*httpURL, *interval, *useEtag, headers.Header)

	go p.Run()
	for {
		select {
		case data := <-p.DataCh:
			log.Println(data)
		case err := <-p.ErrCh:
			log.Println(err)
		}
	}
}
