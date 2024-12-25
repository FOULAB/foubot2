To compile for foubot's hardware, set the following:

	export GOARCH=arm64 GOARM=5 GOOS=linux

Running on a Raspberry Pi Model 4 B with a GPIO wired to the Big Red Button.

More recently, Foubot is wired to a few more things (doorbell, etc), for
details see the wiki:
https://laboratoires.foulab.org/w/tiki-index.php?page=Foubot

I am a horrible person, and do not care for vendoring in this instance!

- Clone repository.
- Within, run ``` $ go build -o foubot2 main.go```

**Click.**

**Boom.**

**Amazebuilds.**
