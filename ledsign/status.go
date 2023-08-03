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
	"foubot2/go-i2c"

	"github.com/mattermost/mattermost-server/v5/model"
)

const StatusEndPoint = configuration.StatusEndPoint
const BotChannel = configuration.BotChannel

type fn func(string, string)

var stateMap = map[byte]bool{
	0x00: false,
	0x01: true,
}

var I2CMu sync.Mutex

type SWITCHSTATE struct {
	ChStop chan struct{}
	once   sync.Once

	Topic string
}

func GetSwitchStatus() (status bool) {
	I2CMu.Lock()
	defer I2CMu.Unlock()

	smb, err := i2c.NewI2C(0x71, 0)
	if err != nil {
		log.Printf("STATUS NewI2C error: %s\n", err)
		return
	}
	smb.Debug = false
	defer smb.Close()

	state := make([]byte, 1)
	_, err = smb.ReadBytes(state)
	if err != nil {
		log.Printf("STATUS ReadBytes error: %s\n", err)
		return
	}

	return stateMap[state[0]]
}

func processStatus(ss *SWITCHSTATE, nc *http.Client, callback fn) {
	var status bool

	first := true

	for {
		select {
		case <-ss.ChStop:
			break
		default:
			newStatus := GetSwitchStatus()
			if first || status != newStatus {
				log.Printf("New status: %v\n", newStatus)
				status = newStatus

				// IRC, Website, Blinker
				match := regexp.MustCompile("\\|\\| LAB (OPEN|CLOSED) \\|\\|").MatchString(ss.Topic)
				if match {
					hold := strings.Split(ss.Topic, "||")

					var strStatus string
					var cmnd string
					if status {
						strStatus = "OPEN"
						cmnd = "On"
					} else {
						strStatus = "CLOSED"
						cmnd = "off"
					}

					topic := fmt.Sprintf("%s|| LAB %s ||%s", hold[0], strStatus, hold[2])

					// Website
					resp, err := nc.Get(StatusEndPoint + strStatus)
					if err != nil {
						log.Printf("StatusEndPoint error: %s\n", err)
					} else {
						io.Copy(ioutil.Discard, resp.Body)
						resp.Body.Close()
					}

					// Blinker
					resp, err = nc.Get(configuration.Blinker + "cm?cmnd=Power%20" + cmnd)
					if err != nil {
						log.Printf("Blinker error: %s\n", err)
					} else {
						io.Copy(ioutil.Discard, resp.Body)
						resp.Body.Close()
					}

					// Melody
					if strStatus == "CLOSED" {
						data := bytes.NewBufferString(`{"jsonrpc": "2.0", "id": 1, "method": "core.playback.stop"}`)
						resp, err = nc.Post("http://melody/mopidy/rpc", "application/json", data)
						if err != nil {
							log.Printf("Melody error: %s\n", err)
						} else {
							io.Copy(ioutil.Discard, resp.Body)
							resp.Body.Close()
						}
					}

					if ss.Topic != topic {
						callback(BotChannel, topic)
						ss.Topic = topic
					}
				}

				// Mattermost
				if configuration.MattermostServer != "" {
					if err := updateMattermost(nc); err != nil {
						log.Printf("Mattermost error: %s\n", err)
					}
				}
			}
			first = false
			time.Sleep(time.Second)
		}
	}
}

func updateMattermost(nc *http.Client) error {
	mm := model.NewAPIv4Client(configuration.MattermostServer)
	mm.HttpClient = nc
	mm.SetToken(configuration.MattermostToken)

	channel, resp := mm.GetChannel(configuration.MattermostChannelId, "")
	if channel == nil {
		return fmt.Errorf("Get channel: %+v", resp)
	}

	const labOpen = "|| LAB OPEN ||"
	const labClosed = "|| LAB CLOSED ||"
	var newHeader string
	if strings.Contains(channel.Header, labOpen) {
		newHeader = strings.ReplaceAll(channel.Header, labOpen, labClosed)
	} else if strings.Contains(channel.Header, labClosed) {
		newHeader = strings.ReplaceAll(channel.Header, labClosed, labOpen)
	} else {
		return fmt.Errorf("Channel header didn't have the key phrase: %q", channel.Header)
	}

	updated, resp := mm.UpdateChannel(&model.Channel{
		Id:     channel.Id,
		Header: newHeader,
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

func NewSwitchStatus(topic string, callback fn) *SWITCHSTATE {
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

	go processStatus(switchInstance, netClient, callback)

	return switchInstance
}
