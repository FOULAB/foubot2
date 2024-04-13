package main

import (
	"fmt"
	"log"
	"regexp"
	"strings"
	"time"

	"foubot2/configuration"
	irc "foubot2/go-ircevent"
	"foubot2/status"
)

const botChannel = configuration.BotChannel
const botNick = configuration.BotNick
const botPswd = configuration.BotPswd
const servertls = configuration.ServerTLS

func handleMessages(event *irc.Event, irc *irc.Connection) {
	target := event.Nick
	prefix := ""
	if event.Arguments[0] == botChannel {
		prefix = fmt.Sprintf("%s: ", target)
		target = botChannel
	}

	command := strings.Split(event.Arguments[1], " ")[0]

	if command == "!vox" && configuration.BotAutoVoice {
		irc.Mode(botChannel, "+v", event.Nick)

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

func handleJoin(event *irc.Event, irc *irc.Connection) {
	go func() {
		time.Sleep(time.Minute * 5)
		irc.Mode(botChannel, "+v", event.Nick)
	}()
}

func handleNick(event *irc.Event, irc *irc.Connection) {
	go func() {
		time.Sleep(time.Minute * 5)
		irc.Mode(botChannel, "+v", event.Nick)
	}()
}

func handlePart(event *irc.Event, irc *irc.Connection) {
	go func() {
		time.Sleep(time.Minute * 5)
		irc.Mode(botChannel, "+v", event.Nick)
	}()
}

func connectOnce() {
	irccon := irc.IRC(botNick, "foubot2")
	irccon.VerboseCallbackHandler = false
	irccon.Debug = false
	irccon.UseTLS = true
	irccon.AutoReconnect = false
	if botPswd != "" {
		irccon.UseSASL = true
		irccon.SASLLogin = botNick
		irccon.SASLPassword = botPswd
	}
	irccon.Server = servertls

	var button *ledsign.SWITCHSTATE
	defer func() {
		if button != nil {
			button.CloseSwitchStatus()
		}
	}()

	irccon.AddCallback("001", func(e *irc.Event) {
		log.Printf("Got welcome, joining %s", botChannel)
		irccon.Join(botChannel)
	})
	irccon.AddCallback("332", func(e *irc.Event) {
		log.Printf("Got topic, starting status goroutine")
		button = ledsign.NewSwitchStatus(e.Arguments[2], irccon)
	})
	irccon.AddCallback("PRIVMSG", func(e *irc.Event) { handleMessages(e, irccon) })
	if configuration.BotAutoVoice {
		irccon.AddCallback("JOIN", func(e *irc.Event) { handleJoin(e, irccon) })
		irccon.AddCallback("NICK", func(e *irc.Event) { handleNick(e, irccon) })
		irccon.AddCallback("PART", func(e *irc.Event) { handlePart(e, irccon) })
	}

	err := irccon.Reconnect()
	if err != nil {
		fmt.Printf("Connect error: %s\n", err)
		return
	}

	irccon.Loop()
}

func main() {
	for {
		connectOnce()
		time.Sleep(60 * time.Second)
	}
}
