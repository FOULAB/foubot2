package ledsign

import (
       "io"
       "io/ioutil"	
       "fmt"
       "time"
       "sync"
       "regexp"
       "strings"
       "net/http"
       
       "../go-i2c"
       "../configuration"
)

type fn func(string, string)

var stateMap = map[byte]bool {
    	           0x00:false,
		   0x01:true,
	       } 

type SWITCHSTATE struct {
     Status bool
     ChStop chan struct{}
     Topic string
     once sync.Once
}


func GetSwitchStatus () (status bool){
     smb, err := i2c.NewI2C(0x71, 0)
     if err != nil {
     	return
     }
     smb.Debug = false
     defer smb.Close()
     
     state := make([]byte, 1)
     _, err = smb.ReadBytes(state)

     return stateMap[state[0]]
}

func processStatus (ss *SWITCHSTATE, nc *http.Client, callback fn) {
     state := GetSwitchStatus()
     strTopic, strStatus := "", ""
     tgtURL := "https://foulab.org/YTDMOWI3N2MXNMEZYWE4MGRHYTRLMZC4NJU5MJI2ZJYYODMYNME5NSAGLQO/"
     hold := []string{}
	
     for {
     	 select {
	  case <-ss.ChStop:
	       break
	  default:
		state = GetSwitchStatus()
		if state != ss.Status {
		   ss.Status = state
		   strTopic = ss.Topic
		   match, _ := regexp.MatchString("\\|\\| LAB (OPEN|CLOSED) \\|\\|", strTopic)
		   if match {
		   	   hold = strings.Split(strTopic, "||")
		   	   if state {
			     strStatus = "OPEN"	   
		   	   } else {
			     strStatus = "CLOSED"
			   }

			   strTopic = fmt.Sprintf("%s|| LAB %s ||%s", hold[0], strStatus, hold[2])

			   resp, _ := nc.Get(tgtURL + strStatus)
			   io.Copy(ioutil.Discard, resp.Body)
			   resp.Body.Close()
			   
		   	   callback(configuration.BotChannel, strTopic)
		   }
		}
		time.Sleep(time.Second)
	 }
     }
}

func (ss SWITCHSTATE) CloseSwitchStatus() {
     ss.once.Do( func() {
	close(ss.ChStop)
     })
}

func NewSwitchStatus(callback fn) (*SWITCHSTATE, error){
     chStop := make(chan struct{})

     netTransport := &http.Transport{
     	     MaxIdleConns:       10,
     	     IdleConnTimeout:    time.Second,
     	     DisableCompression: true,
     }
	
     netClient := &http.Client{
     	     Transport: netTransport,
     	     Timeout: time.Second,
     }

     switchInstance := &SWITCHSTATE{
	     Status: GetSwitchStatus(),
	     ChStop: chStop,
     }
		
     go processStatus(switchInstance, netClient, callback)
     
     return switchInstance, nil
}
