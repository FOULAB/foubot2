package ledsign

import (
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"../configuration"
	"../go-i2c"
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
	Status bool
	ChStop chan struct{}
	once   sync.Once

	mu     sync.Mutex
	Topic  string
}

func (ss *SWITCHSTATE) GetTopic() string {
	ss.mu.Lock()
	defer ss.mu.Unlock()
	return ss.Topic
}

func (ss *SWITCHSTATE) SetTopic(topic string) {
	ss.mu.Lock()
	defer ss.mu.Unlock()
	ss.Topic = topic
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
	var prevTopic string

	for {
		select {
		case <-ss.ChStop:
			break
		default:
			state := GetSwitchStatus()
			strTopic := ss.GetTopic()
			if state != ss.Status || strTopic != prevTopic {
				log.Printf("New status: %v\n", state)
				ss.Status = state
				prevTopic = strTopic
				match, _ := regexp.MatchString("\\|\\| LAB (OPEN|CLOSED) \\|\\|", strTopic)
				if match {
					hold := strings.Split(strTopic, "||")

					var strStatus string
					if state {
						strStatus = "OPEN"
					} else {
						strStatus = "CLOSED"
					}

					strTopic = fmt.Sprintf("%s|| LAB %s ||%s", hold[0], strStatus, hold[2])

					resp, err := nc.Get(StatusEndPoint + strStatus)
					if err != nil {
						log.Printf("StatusEndPoint error: %s\n", err)
					} else {
						io.Copy(ioutil.Discard, resp.Body)
						resp.Body.Close()
					}

					callback(BotChannel, strTopic)
				}
			}
			time.Sleep(time.Second)
		}
	}
}

func (ss SWITCHSTATE) CloseSwitchStatus() {
	ss.once.Do(func() {
		close(ss.ChStop)
	})
}

func NewSwitchStatus(callback fn) (*SWITCHSTATE, error) {
	chStop := make(chan struct{})

	netTransport := &http.Transport{
		MaxIdleConns:       10,
		IdleConnTimeout:    time.Second,
		DisableCompression: true,
	}

	netClient := &http.Client{
		Transport: netTransport,
		Timeout:   3 * time.Second,
	}

	switchInstance := &SWITCHSTATE{
		Status: GetSwitchStatus(),
		ChStop: chStop,
	}

	log.Printf("Starting status: %v\n", switchInstance.Status)

	go processStatus(switchInstance, netClient, callback)

	return switchInstance, nil
}
