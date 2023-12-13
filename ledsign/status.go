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
	irc "foubot2/go-ircevent"

	"github.com/mattermost/mattermost-server/v5/model"
	rpio "github.com/stianeikeland/go-rpio/v4"
)

const StatusEndPoint = configuration.StatusEndPoint
const BotChannel = configuration.BotChannel

var GPIOMu sync.Mutex

type SWITCHSTATE struct {
	ChStop chan struct{}
	once   sync.Once

	Topic string
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
	irccon.AddCallback("TOPIC", func (e *irc.Event) {
		ss.Topic = e.Arguments[1]
		log.Printf("Topic updated manually: %s", ss.Topic)
	})

	for {
		select {
		case <-ss.ChStop:
			break
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

				// IRC topic
				match := regexp.MustCompile("\\|\\| LAB (OPEN|CLOSED) \\|\\|").MatchString(ss.Topic)
				if match {
					hold := strings.Split(ss.Topic, "||")
					topic := fmt.Sprintf("%s|| LAB %s ||%s", hold[0], strStatus, hold[2])
					if ss.Topic != topic {
						if configuration.TopicUseChanserv {
							irccon.Privmsg("ChanServ", fmt.Sprintf("TOPIC %s %s", configuration.BotChannel, topic))
						} else {
							irccon.Topic(BotChannel, topic)
						}
						ss.Topic = topic
					}
				}

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

				// Mattermost
				if configuration.MattermostServer != "" {
					if err := UpdateMattermost(nc, status); err != nil {
						log.Printf("Mattermost error: %s\n", err)
					} else {
						log.Printf("Mattermost updated")
					}
				}
			}
			first = false
			time.Sleep(time.Second)
		}
	}
}

func UpdateMattermost(nc *http.Client, status bool) error {
	mm := model.NewAPIv4Client(configuration.MattermostServer)
	mm.HttpClient = nc
	mm.SetToken(configuration.MattermostToken)

	channel, resp := mm.GetChannel(configuration.MattermostChannelId, "")
	if channel == nil {
		return fmt.Errorf("Get channel: %+v", resp)
	}

	const labOpen = "|| LAB OPEN ||"
	const labClosed = "|| LAB CLOSED ||"

	var strStatus string
	if status {
		strStatus = labOpen
	} else {
		strStatus = labClosed
	}

	var newHeader string
	if strings.Contains(channel.Header, labOpen) {
		newHeader = strings.ReplaceAll(channel.Header, labOpen, strStatus)
	} else if strings.Contains(channel.Header, labClosed) {
		newHeader = strings.ReplaceAll(channel.Header, labClosed, strStatus)
	} else {
		return fmt.Errorf("Channel header didn't have the key phrase: %q", channel.Header)
	}

	log.Printf("New Mattermost header: %q", newHeader)

	updated, resp := mm.PatchChannel(channel.Id, &model.ChannelPatch{
		Header: &newHeader,
	})
	if updated == nil {
		return fmt.Errorf("Update channel: %+v", resp)
	}

	return nil
}

func (ss SWITCHSTATE) CloseSwitchStatus() {
	ss.once.Do(func() {
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
	}

	if err := rpio.Open(); err != nil {
		panic(err)
	}

	go processStatus(switchInstance, netClient, irccon)

	return switchInstance
}
