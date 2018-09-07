package main

import (
	"crypto/tls"
	"fmt"
	"regexp"
	"strings"
	"time"

	"./configuration"
	"./go-ircevent"
	"./ledsign"
)

const botChannel = configuration.BotChannel
const botNick = configuration.BotNick
const botPswd = configuration.BotPswd
const servertls = configuration.ServerTLS

func handleMessages(leds *ledsign.LEDSIGN, event *irc.Event, irc *irc.Connection) {
	target := event.Nick
	prefix := ""
	if event.Arguments[0] == botChannel {
		prefix = fmt.Sprintf("%s: ", target)
		target = botChannel
	}

	command := strings.Split(event.Arguments[1], " ")[0]

	if command == "!sign" && len(strings.Split(event.Arguments[1], " ")) > 1 {
		now := time.Now()
		tid := now.Format("15:04 2-01-2006")

		message := ledsign.SignMsg{
			UserName:  event.Nick,
			Timestamp: tid,
			UserMsg:   event.Arguments[1][6:],
		}

		leds.ChMsgs <- message

		irc.Privmsg(target, fmt.Sprintf("%sAlrity then!", prefix))

		return
	}

	if command == "!status" {
		status := ledsign.GetSwitchStatus()
		if status {
			irc.Privmsg(target, fmt.Sprintf("%sThe lab is currently OPEN.", prefix))
		} else {
			irc.Privmsg(target, fmt.Sprintf("%sSadly, the lab is currently CLOSED.", prefix))
		}

		return
	}

	match, _ := regexp.MatchString(botNick, event.Arguments[1])
	if match {
		irc.Privmsg(target, fmt.Sprintf("%su wot m8?", prefix))
		return
	}

	if event.Arguments[0] != botChannel {
		irc.Privmsg(target, fmt.Sprintf("%sVa?", prefix))
		return
	}
}

func handleTopic(button *ledsign.SWITCHSTATE, event *irc.Event) {
	button.SetTopic(event.Arguments[2])
}

func handleJoin(event *irc.Event, irc *irc.Connection) {
	go func() {
		time.Sleep(time.Minute * 5)
		irc.Mode(botChannel, "+v", event.Nick)
	}()
}

func handleNick(event *irc.Event, irc *irc.Connection) {
	go func() {
		time.Sleep(time.Second * 15)
		irc.Mode(botChannel, "+v", event.Nick)
	}()
}

func handlePart(event *irc.Event, irc *irc.Connection) {
	go func() {
		time.Sleep(time.Minute * 5)
		irc.Mode(botChannel, "+v", event.Nick)
	}()
}

func main() {
	irccon := irc.IRC(botNick, "foubot2")
	irccon.VerboseCallbackHandler = false
	irccon.Debug = false
	irccon.UseTLS = true
	if botPswd != "" {
		irccon.UseSASL = true
		irccon.SASLLogin = botNick
		irccon.SASLPassword = botPswd
	}
	irccon.TLSConfig = &tls.Config{InsecureSkipVerify: true}

	leds, err := ledsign.NewLEDSign()
	defer leds.CloseLEDSign()

	button, err := ledsign.NewSwitchStatus(irccon.Topic)
	defer button.CloseSwitchStatus()

	irccon.AddCallback("001", func(e *irc.Event) { irccon.Join(botChannel) })
	irccon.AddCallback("366", func(e *irc.Event) {})
	irccon.AddCallback("332", func(e *irc.Event) { handleTopic(button, e) })
	irccon.AddCallback("PRIVMSG", func(e *irc.Event) { handleMessages(leds, e, irccon) })
	irccon.AddCallback("JOIN", func(e *irc.Event) { handleJoin(e, irccon) })
	irccon.AddCallback("NICK", func(e *irc.Event) { handleNick(e, irccon) })
	irccon.AddCallback("PART", func(e *irc.Event) { handlePart(e, irccon) })

	err = irccon.Connect(servertls)
	if err != nil {
		fmt.Printf("Connect error: %s\n", err)
		irccon.Quit()
		return
	}

	irccon.Loop()
}
