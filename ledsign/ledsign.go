package ledsign

import (
	"bytes"
	"log"
	"os"
	"sync"
	"time"

	"foubot2/go-i2c"
)

type SignMsg struct {
	UserName  string
	Timestamp string
	UserMsg   string
}

type LEDSIGN struct {
	ChMsgs chan SignMsg
	once   sync.Once
}

var I2CMu sync.Mutex

func recordMessage(sm SignMsg) {
	f, err := os.OpenFile("trace.log", os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0666)
	if err != nil {
		panic(err)
	}
	defer f.Close()

	var line bytes.Buffer
	line.Grow(256)
	line.WriteString(sm.UserName)
	line.WriteString(" ")
	line.WriteString(sm.Timestamp)
	line.WriteString(" ")
	line.WriteString(sm.UserMsg)
	line.WriteString("\n")

	if _, err = f.WriteString(line.String()); err != nil {
		panic(err)
	}
}

func displayMessage(sm SignMsg) {
	I2CMu.Lock()
	defer I2CMu.Unlock()

	smb, err := i2c.NewI2C(0x71, 0)
	if err != nil {
		log.Printf("LEDSIGN NewI2C error: %s\n", err)
		return
	}
	smb.Debug = false
	defer smb.Close()

	var topLine bytes.Buffer
	topLine.Grow(256)
	topLine.WriteString(sm.UserName)
	topLine.WriteString(" @ ")
	topLine.WriteString(sm.Timestamp)
	if topLine.Len() > 255 {
		topLine.Truncate(254)
	}
	topLine.WriteByte(0x00)

	var botLine bytes.Buffer
	botLine.Grow(256)
	botLine.WriteString(sm.UserMsg)
	if botLine.Len() > 255 {
		botLine.Truncate(254)
	}
	botLine.WriteByte(0x00)

	// We need to send the data 32 bytes at a time
	// using the following C structure:
	//       struct lineDataPacket {
	//	      	  uint8 lineID;
	//	      	  uint8 lineData[31];
	//       } linepack;
	// The arduino consumes this until it sees a
	// null terminator, and then posts the message.
	// Max 255 characters per line.

	msgBuf := []byte{}
	hold := []byte{}

	// Send the message to the top line of the sign
	for {
		hold = topLine.Next(31)
		if len(hold) < 31 {
			if len(hold) == 0 {
				break
			}
			hold = append([]byte{0x00}, hold...)
			smb.WriteBytes(hold)
			break
		}
		msgBuf = append([]byte{0x00}, hold...)
		smb.WriteBytes(msgBuf)
	}
	msgBuf = make([]byte, 32, 32)

	// Send the message to the bottom line of the sign
	for {
		hold = botLine.Next(31)
		if len(hold) < 31 {
			if len(hold) == 0 {
				break
			}
			hold = append([]byte{0x01}, hold...)
			smb.WriteBytes(hold)
			break
		}
		msgBuf = append([]byte{0x01}, hold...)
		smb.WriteBytes(msgBuf)
	}
}

func processMessages(ch <-chan SignMsg) {
	for payload := range ch {
		recordMessage(payload)
		displayMessage(payload)
		time.Sleep(time.Second * 5)
	}
}

func (led LEDSIGN) CloseLEDSign() {
	led.once.Do(func() {
		close(led.ChMsgs)
	})
}

func NewLEDSign() (*LEDSIGN, error) {
	chMessages := make(chan SignMsg, 100)

	go processMessages(chMessages)

	instance := &LEDSIGN{ChMsgs: chMessages}

	return instance, nil
}
