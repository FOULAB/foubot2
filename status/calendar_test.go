package ledsign

import (
	"fmt"
	"github.com/jonboulle/clockwork"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

func newServerWithCalendar(time1, time2 time.Time) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `
BEGIN:VCALENDAR

BEGIN:VEVENT
UID:1
DTSTAMP:20060102T150405
DTSTART;TZID="America/New_York":%s
DTEND;TZID="America/New_York":%s
SUMMARY:Event 1
END:VEVENT

BEGIN:VEVENT
UID:2
DTSTAMP:20060102T150405
DTSTART;TZID="America/New_York":%s
DTEND;TZID="America/New_York":%s
SUMMARY:Event 2
END:VEVENT

END:VCALENDAR
`, time1.Format("20060102T150405"), time1.Format("20060102T150405"),
			time2.Format("20060102T150405"), time2.Format("20060102T150405"))
	}))
}

func TestFake(t *testing.T) {
	start := time.Now()
	time1 := start.Add(3 * time.Second)
	time2 := time1.Add(3 * time.Second)

	hs := newServerWithCalendar(time1, time2)

	clock := clockwork.NewRealClock()
	cal := &Calendar{
		Clock:       clock,
		HTTPClient:  hs.Client(),
		URL:         hs.URL,
		GetInterval: 60 * time.Minute,
	}
	cal.Start()
	nextEvent := <-cal.NextEvent
	log.Printf("Next event: %s", nextEvent)
	if nextEvent != "Event 1" {
		t.Errorf("Next event: got %q, want %q", nextEvent, "Event 1")
	}
	startingEvent := <-cal.StartingEvent
	log.Printf("Starting event: %s", nextEvent)
	if startingEvent != "Event 1" {
		t.Errorf("Starting event: got %q, want %q", nextEvent, "Event 1")
	}
	nextEvent = <-cal.NextEvent
	log.Printf("Next event: %s", nextEvent)
	if nextEvent != "Event 2" {
		t.Errorf("Next event: got %q, want %q", nextEvent, "Event 2")
	}
	startingEvent = <-cal.StartingEvent
	log.Printf("Starting event: %s", nextEvent)
	if startingEvent != "Event 2" {
		t.Errorf("Starting event: got %q, want %q", nextEvent, "Event 2")
	}
	nextEvent = <-cal.NextEvent
	log.Printf("Next event: %s", nextEvent)
	if nextEvent != "" {
		t.Errorf("Next event: got %q, want %q", nextEvent, "")
	}
	cal.Close()
}

func TestFakeEarlyClose(t *testing.T) {
	start := time.Now()
	time1 := start.Add(3 * time.Second)
	time2 := time1.Add(3 * time.Second)

	hs := newServerWithCalendar(time1, time2)

	clock := clockwork.NewRealClock()
	cal := &Calendar{
		Clock:       clock,
		HTTPClient:  hs.Client(),
		URL:         hs.URL,
		GetInterval: 60 * time.Minute,
	}
	cal.Start()

	done := make(chan struct{})
	go func() {
		time.Sleep(2 * time.Second)
		// Still events upcoming, but close now.
		cal.Close()
		close(done)
	}()

	nextEvent := <-cal.NextEvent
	log.Printf("Next event: %s", nextEvent)
	if nextEvent != "Event 1" {
		t.Errorf("Next event: got %q, want %q", nextEvent, "Event 1")
	}

	<-done
}

func TestCalendarChange(t *testing.T) {
	start := time.Now()
	time1 := start.Add(3 * time.Second)
	time2 := time1.Add(10 * time.Second)

	calendarMu := sync.Mutex{}
	calendarMu.Lock()
	calendar := fmt.Sprintf(`
BEGIN:VCALENDAR

BEGIN:VEVENT
UID:1
DTSTAMP:20060102T150405
DTSTART;TZID=America/New_York:%s
DTEND;TZID=America/New_York:%s
SUMMARY:Event 1
END:VEVENT

END:VCALENDAR
	`, time1.Format("20060102T150405"), time1.Format("20060102T150405"))
	calendarMu.Unlock()

	hs := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calendarMu.Lock()
		defer calendarMu.Unlock()
		io.WriteString(w, calendar)
	}))

	clock := clockwork.NewRealClock()
	cal := &Calendar{
		Clock:       clock,
		HTTPClient:  hs.Client(),
		URL:         hs.URL,
		GetInterval: 5 * time.Second,
	}
	cal.Start()
	nextEvent := <-cal.NextEvent
	log.Printf("Next event: %s", nextEvent)
	if nextEvent != "Event 1" {
		t.Errorf("Next event: got %q, want %q", nextEvent, "Event 1")
	}
	startingEvent := <-cal.StartingEvent
	log.Printf("Starting event: %s", nextEvent)
	if startingEvent != "Event 1" {
		t.Errorf("Starting event: got %q, want %q", nextEvent, "Event 1")
	}
	nextEvent = <-cal.NextEvent
	log.Printf("Next event: %s", nextEvent)
	if nextEvent != "" {
		t.Errorf("Next event: got %q, want %q", nextEvent, "")
	}

	calendarMu.Lock()
	calendar = fmt.Sprintf(`
BEGIN:VCALENDAR

BEGIN:VEVENT
UID:2
DTSTAMP:20060102T150405
DTSTART;TZID=America/New_York:%s
DTEND;TZID=America/New_York:%s
SUMMARY:Event 2
END:VEVENT

END:VCALENDAR
	`, time2.Format("20060102T150405"), time2.Format("20060102T150405"))
	calendarMu.Unlock()

	nextEvent = <-cal.NextEvent
	log.Printf("Next event: %s", nextEvent)
	if nextEvent != "Event 2" {
		t.Errorf("Next event: got %q, want %q", nextEvent, "Event 2")
	}
	cal.Close()
}

func TestLive(t *testing.T) {
	cal := &Calendar{
		Clock:      clockwork.NewRealClock(),
		HTTPClient: &http.Client{},
		URL:        "https://foulab.org/ical/foulab.ics",
	}
	cal.Start()
	nextEvent := <-cal.NextEvent
	log.Printf("Next event: %s", nextEvent)
	// Can't assert anything, there may be a next event, or not.
	cal.Close()
}
