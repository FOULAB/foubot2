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
	for {
		select {
		case <-ss.ChStop:
			break
		default:
			state := GetSwitchStatus()
			if state != ss.Status {
				log.Printf("New status: %v\n", state)
				ss.Status = state
				match := regexp.MustCompile("\\|\\| LAB (OPEN|CLOSED) \\|\\|").MatchString(ss.Topic)
				if match {
					hold := strings.Split(ss.Topic, "||")

					var strStatus string
					if state {
						strStatus = "OPEN"
					} else {
						strStatus = "CLOSED"
					}

					topic := fmt.Sprintf("%s|| LAB %s ||%s", hold[0], strStatus, hold[2])

					resp, err := nc.Get(StatusEndPoint + strStatus)
					if err != nil {
						log.Printf("StatusEndPoint error: %s\n", err)
					} else {
						io.Copy(ioutil.Discard, resp.Body)
						resp.Body.Close()
					}

					callback(BotChannel, topic)
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

func NewSwitchStatus(callback fn) *SWITCHSTATE {
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

	return switchInstance
}
