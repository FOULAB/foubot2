package ledsign

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"foubot2/configuration"
	irc "github.com/thoj/go-ircevent"

	"github.com/jonboulle/clockwork"
	"github.com/mattermost/mattermost-server/v5/model"
	rpio "github.com/stianeikeland/go-rpio/v4"
)

const StatusEndPoint = configuration.StatusEndPoint
const BotChannel = configuration.BotChannel

type SWITCHSTATE struct {
	ChStop chan struct{}
	once   sync.Once

	Topic    string
	calendar Calendar
}

func GetSwitchStatus() (status bool) {
	pin := rpio.Pin(23)
	pin.Input()
	pin.PullUp()

	return pin.Read() == rpio.High
}

func processStatus(ss *SWITCHSTATE, nc *http.Client, irccon *irc.Connection) {
	var status bool

	first := true

	// If someone changes the topic manually, update our copy.
	irccon.AddCallback("TOPIC", func(e *irc.Event) {
		ss.Topic = e.Arguments[1]
		log.Printf("Topic updated manually: %s", ss.Topic)
	})

OuterLoop:
	for {
		select {
		case <-ss.ChStop:
			break OuterLoop

		case nextEvent := <-ss.calendar.NextEvent:
			// Escape || to prevent interfering with topic structure.
			nextEvent = strings.ReplaceAll(nextEvent, "|", ".")
			if nextEvent == "" {
				nextEvent = "(none)"
			}

			ss.UpdateTopic(irccon, nc, regexp.MustCompile(`\|\| Next event: (.*?) \|\|`), nextEvent)

		case startingEvent := <-ss.calendar.StartingEvent:
			ss.SendMessage(irccon, nc, fmt.Sprintf("Starting event: %s", startingEvent))

		default:
			newStatus := GetSwitchStatus()
			if first || status != newStatus {
				log.Printf("New status: %v\n", newStatus)
				status = newStatus

				var strStatus string
				var cmnd string
				if status {
					strStatus = "OPEN"
					cmnd = "On"
				} else {
					strStatus = "CLOSED"
					cmnd = "off"
				}

				// IRC, Mattermost
				ss.UpdateTopic(irccon, nc, regexp.MustCompile(`\|\| LAB (OPEN|CLOSED) \|\|`), strStatus)

				// IRC announcement (but not at startup, to avoid spam)
				if !first && configuration.TopicSendToChannel {
					irccon.Privmsg(BotChannel, fmt.Sprintf("|| LAB %s ||", strStatus))
				}

				// GPIO
				pin := rpio.Pin(24)
				pin.Output()
				if status {
					pin.High()
				} else {
					pin.Low()
				}

				// For downstairs "Open" LED
				highPin := rpio.Pin(17)
				highPin.Output()
				if status {
					highPin.High()
				} else {
					highPin.Low()
				}

				var resp *http.Response
				var err error

				// Website
				if StatusEndPoint != "" {
					resp, err = nc.Get(StatusEndPoint + strStatus)
					if err != nil {
						log.Printf("StatusEndPoint error: %s\n", err)
					} else {
						log.Printf("StatusEndPoint: %s", resp.Status)
						io.Copy(ioutil.Discard, resp.Body)
						resp.Body.Close()
					}
				}

				// Blinker
				if configuration.Blinker != "" {
					resp, err = nc.Get(configuration.Blinker + "cm?cmnd=Power%20" + cmnd)
					if err != nil {
						log.Printf("Blinker error: %s\n", err)
					} else {
						log.Printf("Blinker: %s", resp.Status)
						io.Copy(ioutil.Discard, resp.Body)
						resp.Body.Close()
					}
				}

				// Melody
				if strStatus == "CLOSED" {
					data := bytes.NewBufferString(`{"jsonrpc": "2.0", "id": 1, "method": "core.playback.stop"}`)
					resp, err = nc.Post("http://melody/mopidy/rpc", "application/json", data)
					if err != nil {
						log.Printf("Melody error: %s\n", err)
					} else {
						log.Printf("Melody: %s", resp.Status)
						io.Copy(ioutil.Discard, resp.Body)
						resp.Body.Close()
					}
				}
			}
			first = false
			time.Sleep(time.Second)
		}
	}
}

// UpdateTopic modifies the topic (IRC, Mattermost) by matching `re` and replacing
// the subexpression by `new`. The regexp must have exactly one subexpression.
func (ss *SWITCHSTATE) UpdateTopic(irccon *irc.Connection, nc *http.Client, re *regexp.Regexp, new string) {
	err := ss.updateTopicIRC(irccon, re, new)
	if err != nil {
		log.Printf("updateTopicIRC error: %s\n", err)
	}

	if configuration.MattermostServer != "" {
		err = ss.updateTopicMattermost(nc, re, new)
		if err != nil {
			log.Printf("updateTopicMattermost error: %s\n", err)
		}
	}
}

func (ss *SWITCHSTATE) updateTopicIRC(irccon *irc.Connection, re *regexp.Regexp, new string) error {
	match := re.FindStringSubmatchIndex(ss.Topic)
	if len(match) == 4 {
		start, end := match[2], match[3]
		topic := ss.Topic[:start] + new + ss.Topic[end:]
		if ss.Topic != topic {
			log.Printf("New IRC topic: %q\n", topic)
			if configuration.TopicUseChanserv {
				irccon.Privmsg("ChanServ", fmt.Sprintf("TOPIC %s %s", configuration.BotChannel, topic))
			} else {
				irccon.SendRawf("TOPIC %s :%s", BotChannel, topic)
			}
			ss.Topic = topic
		} else {
			log.Printf("IRC topic unchanged")
		}
	} else {
		return fmt.Errorf("IRC topic %q did not match regexp %q", ss.Topic, re)
	}
	return nil
}

func (ss *SWITCHSTATE) updateTopicMattermost(nc *http.Client, re *regexp.Regexp, new string) error {
	mm := model.NewAPIv4Client(configuration.MattermostServer)
	mm.HttpClient = nc
	mm.SetToken(configuration.MattermostToken)

	channel, resp := mm.GetChannel(configuration.MattermostChannelId, "")
	if channel == nil {
		log.Printf("Mattermost error: Get channel: %+v\n", resp)
	} else {
		match := re.FindStringSubmatchIndex(channel.Header)
		if len(match) == 4 {
			start, end := match[2], match[3]
			header := channel.Header[:start] + new + channel.Header[end:]

			if header != channel.Header {
				log.Printf("New Mattermost header: %q\n", header)

				updated, resp := mm.PatchChannel(channel.Id, &model.ChannelPatch{
					Header: &header,
				})
				if updated == nil {
					return fmt.Errorf("Patch channel error: %+v", resp)
				}
			} else {
				log.Printf("Mattermost header unchanged\n")
			}
		} else {
			return fmt.Errorf("Mattermost header %q did not match regexp: %q", channel.Header, re)
		}
	}
	return nil
}

func (ss *SWITCHSTATE) SendMessage(irccon *irc.Connection, nc *http.Client, text string) {
	// IRC
	irccon.Privmsg(BotChannel, text)

	// Mattermost
	if configuration.MattermostServer != "" {
		mm := model.NewAPIv4Client(configuration.MattermostServer)
		mm.HttpClient = nc
		mm.SetToken(configuration.MattermostToken)

		post := &model.Post{
			ChannelId: configuration.MattermostChannelId,
			Message:   text,
		}
		post, resp := mm.CreatePost(post)
		if post == nil {
			log.Printf("Create post error: %+v", resp)
		}
	}
}

func (ss SWITCHSTATE) CloseSwitchStatus() {
	ss.once.Do(func() {
		ss.calendar.Close()
		close(ss.ChStop)
	})
}

func NewSwitchStatus(topic string, irccon *irc.Connection) *SWITCHSTATE {
	chStop := make(chan struct{})

	netTransport := &http.Transport{
		MaxIdleConns:       10,
		IdleConnTimeout:    time.Second,
		DisableCompression: true,
	}

	netClient := &http.Client{
		Transport: netTransport,
		Timeout:   10 * time.Second,
	}

	switchInstance := &SWITCHSTATE{
		Topic:  topic,
		ChStop: chStop,
		calendar: Calendar{
			Clock:       clockwork.NewRealClock(),
			HTTPClient:  netClient,
			URL:         configuration.CalendarURL,
			GetInterval: 60 * time.Minute,
		},
	}

	if err := rpio.Open(); err != nil {
		panic(err)
	}

	// For downstairs "Open" LED
	gndPin := rpio.Pin(21)
	gndPin.Output()
	gndPin.Low()

	switchInstance.calendar.Start()

	go processStatus(switchInstance, netClient, irccon)

	return switchInstance
}
