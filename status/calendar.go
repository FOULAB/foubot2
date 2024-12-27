package ledsign

import (
	ical "github.com/arran4/golang-ical"
	"github.com/jonboulle/clockwork"
	"io"
	"log"
	"net/http"
	"sort"
	"sync"
	"time"
)

type Calendar struct {
	Clock      clockwork.Clock
	HTTPClient *http.Client
	URL        string

	NextEvent     chan string
	StartingEvent chan string

	wgGet     sync.WaitGroup
	stopGet   chan struct{}
	wgTimer   sync.WaitGroup
	stopTimer chan struct{}
}

type eventByStartAt []*ical.VEvent

func (v eventByStartAt) Len() int      { return len(v) }
func (v eventByStartAt) Swap(i, j int) { v[i], v[j] = v[j], v[i] }
func (v eventByStartAt) Less(i, j int) bool {
	starti, err := v[i].GetStartAt()
	if err != nil {
		panic(err)
	}
	startj, err := v[j].GetStartAt()
	if err != nil {
		panic(err)
	}
	return starti.Before(startj)
}

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
		log.Printf("Calendar sleeping %s\n", sleep)
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
			log.Printf("Calendar get: %s\n", err)
			sleep = 1 * time.Minute
			continue
		}

		log.Printf("Calendar response %d (length %s)\n", resp.StatusCode, resp.Header.Get("Content-Length"))
		switch resp.StatusCode {
		case http.StatusOK:
			err = c.parse(resp.Body)
			if err == nil {
				lastModified = resp.Header.Get("Last-Modified")
				log.Printf("Last modified now: %s\n", lastModified)
				sleep = 60 * time.Minute
			} else {
				sleep = 1 * time.Minute
			}
			resp.Body.Close()
		case http.StatusNotModified:
			sleep = 60 * time.Minute
		default:
			log.Printf("Unexpected response %d\n", resp.StatusCode)
			sleep = 1 * time.Minute
		}
	}
}

func (c *Calendar) parse(r io.Reader) error {
	cal, err := ical.ParseCalendar(r)
	if err != nil {
		log.Printf("Parse calendar error: %s", err)
		return err
	}

	events := cal.Events()

	log.Printf("Calendar parsed %d events\n", len(events))

	// TODO: avoid sorting events in the past
	sort.Sort(eventByStartAt(events))

	if c.stopTimer != nil {
		close(c.stopTimer)
		c.wgTimer.Wait()
	}

	c.stopTimer = make(chan struct{})
	c.wgTimer.Add(1)
	go c.timerLoop(events)
	return nil
}

func getStartAtOrPanic(e *ical.VEvent) time.Time {
	start, err := e.GetStartAt()
	if err != nil {
		log.Panicf("GetStartAt: %s\n", err)
	}
	return start
}

func (c *Calendar) timerLoop(events []*ical.VEvent) {
	defer c.wgTimer.Done()

	i := 0
	for {
		// TODO: make this robust against system time changes
		now := c.Clock.Now()

		// TODO: free memory for past events
		for i < len(events) && getStartAtOrPanic(events[i]).Before(now) {
			i++
		}

		log.Printf("Events in the future: %d\n", len(events[i:]))

		if i < len(events) {
			log.Printf("Next event: %s\n", events[i])
			c.NextEvent <- events[i].GetProperty(ical.ComponentPropertySummary).Value
		} else {
			log.Printf("Next event: (none)\n")
			c.NextEvent <- ""
			log.Printf("Out of events, timer loop exiting")
			return
		}

		select {
		case <-c.stopTimer:
			log.Printf("Timer loop stopping")
			return
		case <-c.Clock.After(getStartAtOrPanic(events[i]).Sub(now)):
		}

		log.Printf("Starting event: %s\n", events[i])
		c.StartingEvent <- events[i].GetProperty(ical.ComponentPropertySummary).Value

		i++
	}
}

func (c *Calendar) Close() {
	close(c.stopTimer)
	c.wgTimer.Wait()
	close(c.stopGet)
	c.wgGet.Wait()
}
