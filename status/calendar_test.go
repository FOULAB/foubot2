package ledsign

import (
	"fmt"
	"github.com/jonboulle/clockwork"
	"log"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestFake(t *testing.T) {
	start := time.Now()
	time1 := start.Add(3 * time.Second)
	time2 := time1.Add(3 * time.Second)

	hs := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `
BEGIN:VCALENDAR

BEGIN:VEVENT
DTSTART;TZID=America/New_York:%s
DTEND;TZID=America/New_York:%s
SUMMARY:Event 1
END:VEVENT

BEGIN:VEVENT
DTSTART;TZID=America/New_York:%s
DTEND;TZID=America/New_York:%s
SUMMARY:Event 2
END:VEVENT

END:VCALENDAR
`, time1.Format("20060102T150405"), time1.Format("20060102T150405"),
			time2.Format("20060102T150405"), time2.Format("20060102T150405"))
	}))

	clock := clockwork.NewRealClock()
	cal := &Calendar{
		Clock:      clock,
		HTTPClient: hs.Client(),
		URL:        hs.URL,
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
