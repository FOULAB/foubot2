package ledsign

import (
	"github.com/apognu/gocal"
	"github.com/jonboulle/clockwork"
	"io"
	"log"
	"net/http"
	"sort"
	"sync"
	"time"
)

type Calendar struct {
	Clock       clockwork.Clock
	HTTPClient  *http.Client
	URL         string
	GetInterval time.Duration

	NextEvent     chan string
	StartingEvent chan string

	wgGet     sync.WaitGroup
	stopGet   chan struct{}
	wgTimer   sync.WaitGroup
	stopTimer chan struct{}
}

type eventsByStart []gocal.Event

func (v eventsByStart) Len() int           { return len(v) }
func (v eventsByStart) Swap(i, j int)      { v[i], v[j] = v[j], v[i] }
func (v eventsByStart) Less(i, j int) bool { return v[i].Start.Before(*v[j].Start) }

func (c *Calendar) Start() {
	c.NextEvent = make(chan string)
	c.StartingEvent = make(chan string)
	c.stopGet = make(chan struct{})
	c.wgGet.Add(1)
	go c.getLoop()
}

func (c *Calendar) getLoop() {
	var sleep time.Duration
	var lastModified string
	defer c.wgGet.Done()
	for {
		log.Printf("Calendar sleeping %s", sleep)
		select {
		case <-c.stopGet:
			return
		case <-c.Clock.After(sleep):
		}

		req, err := http.NewRequest("GET", c.URL, nil)
		if err != nil {
			log.Panicf("NewRequest: %s", err)
		}
		if lastModified != "" {
			req.Header.Set("If-Modified-Since", lastModified)
		}
		resp, err := c.HTTPClient.Do(req)
		if err != nil {
			log.Printf("Calendar get: %s", err)
			sleep = 1 * time.Minute
			continue
		}

		log.Printf("Calendar response %d (length %s)", resp.StatusCode, resp.Header.Get("Content-Length"))
		switch resp.StatusCode {
		case http.StatusOK:
			err = c.parse(resp.Body)
			if err == nil {
				lastModified = resp.Header.Get("Last-Modified")
				log.Printf("Last modified now: %s", lastModified)
				sleep = c.GetInterval
			} else {
				sleep = 1 * time.Minute
			}
			resp.Body.Close()
		case http.StatusNotModified:
			sleep = c.GetInterval
		default:
			log.Printf("Unexpected response %d", resp.StatusCode)
			sleep = 1 * time.Minute
		}
	}
}

func (c *Calendar) parse(r io.Reader) error {
	// Don't use https://github.com/arran4/golang-ical, it doesn't handle non-standard
	// properties (X-...) which we use in our calendar (X-EVENTMONTH:JAN2025 -
	// https://github.com/FOULAB/foulab-dot-org-hugo/blob/91dfe26930ac1f56b10b645a4979d480f941f647/static/ical/foulab.ics#L1299)

	start, end := time.Now(), time.Now().Add(30*24*time.Hour)
	cal := gocal.NewParser(r)
	cal.Start, cal.End = &start, &end
	if err := cal.Parse(); err != nil {
		log.Printf("Parse calendar error: %s", err)
		return err
	}

	events := cal.Events
	log.Printf("Calendar parsed %d events", len(events))

	// TODO: avoid sorting events in the past
	sort.Sort(eventsByStart(events))

	// TODO: add 'morning' timer ("Events tonight: ...")

	if c.stopTimer != nil {
		close(c.stopTimer)
		c.wgTimer.Wait()
	}

	c.stopTimer = make(chan struct{})
	c.wgTimer.Add(1)
	go c.timerLoop(events)
	return nil
}

func (c *Calendar) timerLoop(events []gocal.Event) {
	defer c.wgTimer.Done()

	i := 0
	for {
		// TODO: make this robust against system time changes
		now := c.Clock.Now()

		// Past events will remain in memory until timerLoop is restarted (eg. due to
		// ics file changed).
		for i < len(events) && events[i].Start.Before(now) {
			i++
		}

		log.Printf("Events in the future: %d", len(events[i:]))

		if i < len(events) {
			log.Printf("Next event: %+v", events[i])
			c.NextEvent <- events[i].Summary
		} else {
			log.Printf("Next event: (none)")
			c.NextEvent <- ""
			log.Printf("Out of events, timer loop exiting")
			return
		}

		select {
		case <-c.stopTimer:
			log.Printf("Timer loop stopping")
			return
		case <-c.Clock.After(events[i].Start.Sub(now)):
		}

		log.Printf("Starting event: %+v", events[i])
		c.StartingEvent <- events[i].Summary

		i++
	}
}

func (c *Calendar) Close() {
	close(c.stopTimer)
	c.wgTimer.Wait()
	close(c.stopGet)
	c.wgGet.Wait()
}
