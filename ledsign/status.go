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
	Topic  string
	once   sync.Once
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
	state := GetSwitchStatus()
	log.Printf("Starting status: %v\n", state)

	for {
		select {
		case <-ss.ChStop:
			break
		default:
			state := GetSwitchStatus()
			if state != ss.Status {
				log.Printf("New status: %v\n", state)
				ss.Status = state
				strTopic := ss.Topic
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

					resp, _ := nc.Get(StatusEndPoint + strStatus)
					io.Copy(ioutil.Discard, resp.Body)
					resp.Body.Close()

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
		Timeout:   time.Second,
	}

	switchInstance := &SWITCHSTATE{
		Status: GetSwitchStatus(),
		ChStop: chStop,
	}

	go processStatus(switchInstance, netClient, callback)

	return switchInstance, nil
}
